package main

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/bbloom"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

type cidSet interface {
	Add(cid.Cid)
	Has(cid.Cid) bool
	HasRaw([]byte) bool
	Len() int
}

type bloomSet struct {
	bloom *bbloom.Bloom
}

func newBloomSet(size int64) (*bloomSet, error) {
	b, err := bbloom.New(float64(size), 3)
	if err != nil {
		return nil, err
	}

	return &bloomSet{bloom: b}, nil
}

func (bs *bloomSet) Add(c cid.Cid) {
	bs.bloom.Add(c.Hash())

}

func (bs *bloomSet) Has(c cid.Cid) bool {
	return bs.bloom.Has(c.Hash())
}

func (bs *bloomSet) HasRaw(b []byte) bool {
	return bs.bloom.Has(b)
}

func (bs *bloomSet) Len() int {
	return int(bs.bloom.ElementsAdded())
}

type mapSet struct {
	m map[string]struct{}
}

func newMapSet() *mapSet {
	return &mapSet{m: make(map[string]struct{})}
}

func (bs *mapSet) Add(c cid.Cid) {
	bs.m[string(c.Hash())] = struct{}{}
}

func (bs *mapSet) Has(c cid.Cid) bool {
	_, ok := bs.m[string(c.Hash())]
	return ok
}

func (bs *mapSet) HasRaw(b []byte) bool {
	_, ok := bs.m[string(b)]
	return ok
}

func (bs *mapSet) Len() int {
	return len(bs.m)
}

var stateTreePruneCmd = &cli.Command{
	Name:        "state-prune",
	Description: "Deletes old state root data from local chainstore",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.Int64Flag{
			Name:  "keep-from-lookback",
			Usage: "keep stateroots at or newer than the current height minus this lookback",
			Value: 1800, // 2 x finality
		},
		&cli.IntFlag{
			Name:  "delete-up-to",
			Usage: "delete up to the given number of objects (used to run a faster 'partial' sync)",
		},
		&cli.BoolFlag{
			Name:  "use-bloom-set",
			Usage: "use a bloom filter for the 'good' set instead of a map, reduces memory usage but may not clean up as much",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "only enumerate the good set, don't do any deletions",
		},
		&cli.BoolFlag{
			Name:  "only-ds-gc",
			Usage: "Only run datastore GC",
		},
		&cli.IntFlag{
			Name:  "gc-count",
			Usage: "number of times to run gc",
			Value: 20,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		fsrepo, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		lkrepo, err := fsrepo.Lock(repo.FullNode)
		if err != nil {
			return err
		}

		defer lkrepo.Close() //nolint:errcheck

		ds, err := lkrepo.Datastore("/chain")
		if err != nil {
			return err
		}

		defer ds.Close() //nolint:errcheck

		mds, err := lkrepo.Datastore("/metadata")
		if err != nil {
			return err
		}
		defer mds.Close() //nolint:errcheck

		if cctx.Bool("only-ds-gc") {
			gcds, ok := ds.(datastore.GCDatastore)
			if ok {
				fmt.Println("running datastore gc....")
				for i := 0; i < cctx.Int("gc-count"); i++ {
					if err := gcds.CollectGarbage(); err != nil {
						return xerrors.Errorf("datastore GC failed: %w", err)
					}
				}
				fmt.Println("gc complete!")
				return nil
			}
			return fmt.Errorf("datastore doesnt support gc")
		}

		bs := blockstore.NewBlockstore(ds)

		cs := store.NewChainStore(bs, mds, vm.Syscalls(ffiwrapper.ProofVerifier), nil)
		if err := cs.Load(); err != nil {
			return fmt.Errorf("loading chainstore: %w", err)
		}

		var goodSet cidSet
		if cctx.Bool("use-bloom-set") {
			bset, err := newBloomSet(10000000)
			if err != nil {
				return err
			}
			goodSet = bset
		} else {
			goodSet = newMapSet()
		}

		ts := cs.GetHeaviestTipSet()

		rrLb := abi.ChainEpoch(cctx.Int64("keep-from-lookback"))

		if err := cs.WalkSnapshot(ctx, ts, rrLb, true, func(c cid.Cid) error {
			if goodSet.Len()%20 == 0 {
				fmt.Printf("\renumerating keep set: %d             ", goodSet.Len())
			}
			goodSet.Add(c)
			return nil
		}); err != nil {
			return fmt.Errorf("snapshot walk failed: %w", err)
		}

		fmt.Println()
		fmt.Printf("Successfully marked keep set! (%d objects)\n", goodSet.Len())

		if cctx.Bool("dry-run") {
			return nil
		}

		var b datastore.Batch
		var batchCount int
		markForRemoval := func(c cid.Cid) error {
			if b == nil {
				nb, err := ds.Batch()
				if err != nil {
					return fmt.Errorf("opening batch: %w", err)
				}

				b = nb
			}
			batchCount++

			if err := b.Delete(dshelp.MultihashToDsKey(c.Hash())); err != nil {
				return err
			}

			if batchCount > 100 {
				if err := b.Commit(); err != nil {
					return xerrors.Errorf("failed to commit batch deletes: %w", err)
				}
				b = nil
				batchCount = 0
			}
			return nil
		}

		res, err := ds.Query(query.Query{KeysOnly: true})
		if err != nil {
			return xerrors.Errorf("failed to query datastore: %w", err)
		}

		dupTo := cctx.Int("delete-up-to")

		var deleteCount int
		var goodHits int
		for {
			v, ok := res.NextSync()
			if !ok {
				break
			}

			bk, err := dshelp.BinaryFromDsKey(datastore.RawKey(v.Key[len("/blocks"):]))
			if err != nil {
				return xerrors.Errorf("failed to parse key: %w", err)
			}

			if goodSet.HasRaw(bk) {
				goodHits++
				continue
			}

			nc := cid.NewCidV1(cid.Raw, bk)

			deleteCount++
			if err := markForRemoval(nc); err != nil {
				return fmt.Errorf("failed to remove cid %s: %w", nc, err)
			}

			if deleteCount%20 == 0 {
				fmt.Printf("\rdeleting %d objects (good hits: %d)...      ", deleteCount, goodHits)
			}

			if dupTo != 0 && deleteCount > dupTo {
				break
			}
		}

		if b != nil {
			if err := b.Commit(); err != nil {
				return xerrors.Errorf("failed to commit final batch delete: %w", err)
			}
		}

		gcds, ok := ds.(datastore.GCDatastore)
		if ok {
			fmt.Println("running datastore gc....")
			for i := 0; i < cctx.Int("gc-count"); i++ {
				if err := gcds.CollectGarbage(); err != nil {
					return xerrors.Errorf("datastore GC failed: %w", err)
				}
			}
			fmt.Println("gc complete!")
		}

		return nil
	},
}
