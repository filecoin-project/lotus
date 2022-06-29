package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/pb"
	"github.com/dustin/go-humanize"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-car"
	"github.com/multiformats/go-base32"
	mh "github.com/multiformats/go-multihash"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/lotus-shed/shedgen"
	"github.com/filecoin-project/lotus/node/repo"
)

var exportChainCmd = &cli.Command{
	Name:        "export",
	Description: "Export chain from repo (requires node to be offline)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "tipset to export from",
		},
		&cli.Int64Flag{
			Name: "recent-stateroots",
		},
		&cli.BoolFlag{
			Name: "full-state",
		},
		&cli.BoolFlag{
			Name: "skip-old-msgs",
		},
	},
	Subcommands: []*cli.Command{
		exportRawCmd,
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return lcli.ShowHelp(cctx, fmt.Errorf("must specify file name to write export to"))
		}

		ctx := context.TODO()

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.FullNode)
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		fi, err := os.Create(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("opening the output file: %w", err)
		}

		defer fi.Close() //nolint:errcheck

		bs, err := lr.Blockstore(ctx, repo.UniversalBlockstore)
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

		mds, err := lr.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		cs := store.NewChainStore(bs, bs, mds, nil, nil)
		defer cs.Close() //nolint:errcheck

		if err := cs.Load(context.Background()); err != nil {
			return err
		}

		nroots := abi.ChainEpoch(cctx.Int64("recent-stateroots"))
		fullstate := cctx.Bool("full-state")
		skipoldmsgs := cctx.Bool("skip-old-msgs")

		var ts *types.TipSet
		if tss := cctx.String("tipset"); tss != "" {
			cids, err := lcli.ParseTipSetString(tss)
			if err != nil {
				return xerrors.Errorf("failed to parse tipset (%q): %w", tss, err)
			}

			tsk := types.NewTipSetKey(cids...)

			selts, err := cs.LoadTipSet(context.Background(), tsk)
			if err != nil {
				return xerrors.Errorf("loading tipset: %w", err)
			}
			ts = selts
		} else {
			ts = cs.GetHeaviestTipSet()
		}

		if fullstate {
			nroots = ts.Height() + 1
		}

		if err := cs.Export(ctx, ts, nroots, skipoldmsgs, fi); err != nil {
			return xerrors.Errorf("export failed: %w", err)
		}

		return nil
	},
}

var exportRawCmd = &cli.Command{
	Name:        "raw",
	Description: "Export raw blocks from repo (requires node to be offline)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.StringFlag{
			Name:  "car-size",
			Value: "50M",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return lcli.ShowHelp(cctx, fmt.Errorf("must specify file name to write export to"))
		}

		ctx := context.TODO()

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.LockRO(repo.FullNode)
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		out := cctx.Args().First()
		err = os.Mkdir(out, 0755)
		if err != nil {
			return xerrors.Errorf("creating output dir: %w", err)
		}

		maxSz, err := humanize.ParseBytes(cctx.String("car-size"))
		if err != nil {
			return xerrors.Errorf("parse --car-size: %w", err)
		}

		cars := 0

		carb := &rawCarb{
			max:    maxSz,
			blocks: map[cid.Cid]block.Block{},
		}

		{
			consume := func(c cid.Cid, b block.Block) error {
				err = carb.consume(c, b)
				switch err {
				case nil:
				case errFullCar:
					root, err := carb.finalize()
					if err != nil {
						return xerrors.Errorf("carb finalize: %w", err)
					}

					if err := carb.writeCar(ctx, filepath.Join(out, fmt.Sprintf("chain%d.car", cars)), root); err != nil {
						return xerrors.Errorf("writeCar: %w", err)
					}

					cars++

					if cars > 10 {
						return xerrors.Errorf("enough")
					}

					carb = &rawCarb{
						max:    maxSz,
						blocks: map[cid.Cid]block.Block{},
					}

					log.Infow("gc")
					go runtime.GC()

				default:
					return xerrors.Errorf("carb consume: %w", err)
				}
				return nil
			}

			{
				path := filepath.Join(lr.Path(), "datastore", "chain")
				opts, err := repo.BadgerBlockstoreOptions(repo.UniversalBlockstore, path, false)
				if err != nil {
					return err
				}

				opts.Logger = &badgerLog{
					SugaredLogger: log.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar(),
					skip2:         log.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar(),
				}

				log.Infow("open db")

				db, err := badger.Open(opts.Options)
				if err != nil {
					return fmt.Errorf("failed to open badger blockstore: %w", err)
				}
				defer db.Close() // nolint:errcheck

				log.Infow("new stream")

				var wlk sync.Mutex

				str := db.NewStream()
				str.NumGo = 16
				str.LogPrefix = "bstream"
				str.Send = func(list *pb.KVList) (err error) {
					defer func() {
						if err != nil {
							log.Errorw("send error", "err", err)
						}
					}()

					for _, kv := range list.Kv {
						if kv.Key == nil || kv.Value == nil {
							continue
						}
						if !strings.HasPrefix(string(kv.Key), "/blocks/") {
							log.Infow("no blocks prefix", "key", string(kv.Key))
							continue
						}

						h, err := base32.RawStdEncoding.DecodeString(string(kv.Key[len("/blocks/"):]))
						if err != nil {
							return xerrors.Errorf("decode b32 ds key %x: %w", kv.Key, err)
						}

						c := cid.NewCidV1(cid.Raw, h)

						b, err := block.NewBlockWithCid(kv.Value, c)
						if err != nil {
							return xerrors.Errorf("readblk: %w", err)
						}

						wlk.Lock()
						err = consume(c, b)
						wlk.Unlock()
						if err != nil {
							return xerrors.Errorf("consume stream block: %w", err)
						}
					}

					return nil
				}

				if err := str.Orchestrate(ctx); err != nil {
					return xerrors.Errorf("orchestrate stream: %w", err)
				}
			}
		}

		log.Infow("write last")

		root, err := carb.finalize()
		if err != nil {
			return xerrors.Errorf("carb finalize: %w", err)
		}

		if err := carb.writeCar(ctx, filepath.Join(out, fmt.Sprintf("chain%d.car", cars)), root); err != nil {
			return xerrors.Errorf("writeCar: %w", err)
		}

		return nil
	},
}

var errFullCar = errors.New("full")

const maxlinks = 16

type rawCarb struct {
	blockstore.Blockstore

	max, cur uint64

	nodes []*shedgen.CarbNode

	blocks map[cid.Cid]block.Block
}

func (rc *rawCarb) Has(ctx context.Context, c cid.Cid) (bool, error) {
	_, has := rc.blocks[c]
	return has, nil
}

func (rc *rawCarb) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	b, has := rc.blocks[c]
	if !has {
		return nil, ipld.ErrNotFound{Cid: c}
	}
	return b, nil
}

func (rc *rawCarb) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	b, has := rc.blocks[c]
	if !has {
		return 0, ipld.ErrNotFound{Cid: c}
	}
	return len(b.RawData()), nil
}

func (rc *rawCarb) checkNodes(maxl int) error {
	if len(rc.nodes) == 0 {
		log.Infow("add level", "l", 0)
		rc.nodes = append(rc.nodes, new(shedgen.CarbNode))
	}
	for i := 0; i < len(rc.nodes); i++ {
		if len(rc.nodes[i].Sub) <= maxl {
			break
		}
		if len(rc.nodes) <= i+1 {
			log.Infow("add level", "l", i+1)
			rc.nodes = append(rc.nodes, new(shedgen.CarbNode))
		}

		var bb bytes.Buffer
		if err := rc.nodes[i].MarshalCBOR(&bb); err != nil {
			return err
		}
		c, err := cid.Prefix{
			Version:  1,
			Codec:    cid.DagCBOR,
			MhType:   mh.SHA2_256,
			MhLength: -1,
		}.Sum(bb.Bytes())
		if err != nil {
			return xerrors.Errorf("gen cid: %w", err)
		}

		b, err := block.NewBlockWithCid(bb.Bytes(), c)
		if err != nil {
			return xerrors.Errorf("new block: %w", err)
		}

		if i > 1 {
			log.Infow("compact", "from", i, "to", i+1, "sub", c.String())
		}

		rc.nodes[i+1].Sub = append(rc.nodes[i+1].Sub, c)
		rc.blocks[c] = b
		rc.nodes[i] = new(shedgen.CarbNode)
		rc.cur += uint64(bb.Len())
	}

	return nil
}

func (rc *rawCarb) consume(c cid.Cid, b block.Block) error {
	if err := rc.checkNodes(maxlinks); err != nil {
		return err
	}
	if rc.cur+uint64(len(b.RawData())) > rc.max {
		return errFullCar
	}

	rc.cur += uint64(len(b.RawData()))

	b, err := block.NewBlockWithCid(b.RawData(), c)
	if err != nil {
		return xerrors.Errorf("create raw block: %w", err)
	}

	rc.blocks[c] = b
	rc.nodes[0].Sub = append(rc.nodes[0].Sub, c)

	return nil
}

func (rc *rawCarb) finalize() (cid.Cid, error) {
	if len(rc.nodes) == 0 {
		rc.nodes = append(rc.nodes, new(shedgen.CarbNode))
	}

	for i := 0; i < len(rc.nodes); i++ {
		var bb bytes.Buffer
		if err := rc.nodes[i].MarshalCBOR(&bb); err != nil {
			return cid.Undef, err
		}
		c, err := cid.Prefix{
			Version:  1,
			Codec:    cid.DagCBOR,
			MhType:   mh.SHA2_256,
			MhLength: -1,
		}.Sum(bb.Bytes())
		if err != nil {
			return cid.Undef, xerrors.Errorf("gen cid: %w", err)
		}

		b, err := block.NewBlockWithCid(bb.Bytes(), c)
		if err != nil {
			return cid.Undef, xerrors.Errorf("new block: %w", err)
		}

		log.Infow("fin", "level", i, "cid", c.String())

		rc.blocks[c] = b
		rc.nodes[i] = new(shedgen.CarbNode)
		rc.cur += uint64(bb.Len())

		if len(rc.nodes[i].Sub) <= 1 && i == len(rc.nodes)-1 {
			return c, err
		}
		if len(rc.nodes) <= i+1 {
			rc.nodes = append(rc.nodes, new(shedgen.CarbNode))
		}
		rc.nodes[i+1].Sub = append(rc.nodes[i+1].Sub, c)
	}
	return cid.Undef, xerrors.Errorf("failed to finalize")
}

func (rc *rawCarb) writeCar(ctx context.Context, path string, root cid.Cid) error {
	f, err := os.Create(path)
	if err != nil {
		return xerrors.Errorf("create out car: %w", err)
	}

	bs := rc
	ds := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

	log.Infow("write car", "path", path, "root", root.String(), "blocks", len(rc.blocks))

	return car.WriteCar(ctx, ds, []cid.Cid{root}, f)
}

var _ blockstore.Blockstore = &rawCarb{}

type badgerLog struct {
	*zap.SugaredLogger
	skip2 *zap.SugaredLogger
}

func (b *badgerLog) Warningf(format string, args ...interface{}) {
	b.skip2.Warnf(format, args...)
}
