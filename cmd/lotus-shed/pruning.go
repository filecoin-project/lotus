package main

import (
	"context"
	"fmt"
	"io"

	"github.com/ipfs/bbloom"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/repo"
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

		bs, err := lkrepo.Blockstore(ctx, repo.UniversalBlockstore)
		if err != nil {
			return fmt.Errorf("failed to open blockstore: %w", err)
		}

		defer func() {
			if c, ok := bs.(io.Closer); ok {
				if err := c.Close(); err != nil {
					log.Warnf("failed to close blockstore: %s", err)
				}
			}
		}()

		mds, err := lkrepo.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}
		defer mds.Close() //nolint:errcheck

		const DiscardRatio = 0.2
		if cctx.Bool("only-ds-gc") {
			fmt.Println("running datastore gc....")
			gbs, ok := bs.(blockstore.BlockstoreGCOnce)
			if !ok {
				return xerrors.Errorf("blockstore %T does not support GC", bs)
			}
			for i := 0; i < cctx.Int("gc-count"); i++ {
				if err := gbs.GCOnce(ctx, blockstore.WithThreshold(DiscardRatio)); err != nil {
					return xerrors.Errorf("datastore GC failed: %w", err)
				}
			}
			fmt.Println("gc complete!")
			return nil
		}

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		if err := cs.Load(context.Background()); err != nil {
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

		if err := cs.WalkSnapshot(ctx, ts, rrLb, true, true, func(c cid.Cid) error {
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

		keys, err := bs.AllKeysChan(context.Background())
		if err != nil {
			return xerrors.Errorf("failed to query blockstore: %w", err)
		}

		dupTo := cctx.Int("delete-up-to")

		var deleteCount int
		var goodHits int
		for k := range keys {
			if goodSet.Has(k) {
				goodHits++
				continue
			}

			if err := bs.DeleteBlock(ctx, k); err != nil {
				return fmt.Errorf("failed to remove cid %s: %w", k, err)
			}

			if deleteCount%20 == 0 {
				fmt.Printf("\rdeleting %d objects (good hits: %d)...      ", deleteCount, goodHits)
			}

			if dupTo != 0 && deleteCount > dupTo {
				break
			}
		}

		fmt.Println("running datastore gc....")
		gbs, ok := bs.(blockstore.BlockstoreGCOnce)
		if !ok {
			fmt.Println("gc not supported...")
			return nil
		}

		for i := 0; i < cctx.Int("gc-count"); i++ {
			if err := gbs.GCOnce(ctx, blockstore.WithThreshold(DiscardRatio)); err != nil {
				return xerrors.Errorf("datastore GC failed: %w", err)
			}
		}
		fmt.Println("gc complete!")

		return nil
	},
}
