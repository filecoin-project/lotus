package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/filecoin-project/lotus/chain/actors/builtin"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/lotus/chain/actors/builtin/market"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/repo"
	miner2 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
)

var terminationsCmd = &cli.Command{
	Name:        "terminations",
	Description: "Lists terminated deals from the past 2 days",
	ArgsUsage:   "[block to look back from] [lookback period (epochs)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		if cctx.NArg() != 2 {
			return fmt.Errorf("must pass block cid && lookback period")
		}

		blkCid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

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

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		cst := cbor.NewCborStore(bs)
		store := adt.WrapStore(ctx, cst)

		blk, err := cs.GetBlock(blkCid)
		if err != nil {
			return err
		}

		lbp, err := strconv.Atoi(cctx.Args().Get(1))
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		cutoff := blk.Height - abi.ChainEpoch(lbp)

		for blk.Height > cutoff {
			pts, err := cs.LoadTipSet(types.NewTipSetKey(blk.Parents...))
			if err != nil {
				return err
			}

			blk = pts.Blocks()[0]

			msgs, err := cs.MessagesForTipset(pts)
			if err != nil {
				return err
			}

			for _, v := range msgs {
				msg := v.VMMessage()
				if msg.Method != miner.Methods.TerminateSectors {
					continue
				}

				tree, err := state.LoadStateTree(cst, blk.ParentStateRoot)
				if err != nil {
					return err
				}

				minerAct, err := tree.GetActor(msg.To)
				if err != nil {
					return err
				}

				if !builtin.IsStorageMinerActor(minerAct.Code) {
					continue
				}

				minerSt, err := miner.Load(store, minerAct)
				if err != nil {
					return err
				}

				marketAct, err := tree.GetActor(market.Address)
				if err != nil {
					return err
				}

				marketSt, err := market.Load(store, marketAct)
				if err != nil {
					return err
				}

				proposals, err := marketSt.Proposals()
				if err != nil {
					return err
				}

				var termParams miner2.TerminateSectorsParams
				err = termParams.UnmarshalCBOR(bytes.NewBuffer(msg.Params))
				if err != nil {
					return err
				}

				for _, t := range termParams.Terminations {
					sectors, err := minerSt.LoadSectors(&t.Sectors)
					if err != nil {
						return err
					}

					for _, sector := range sectors {
						for _, deal := range sector.DealIDs {
							prop, find, err := proposals.Get(deal)
							if err != nil {
								return err
							}
							if find {
								fmt.Printf("%s, %d, %d, %s, %s, %s\n", msg.To, sector.SectorNumber, deal, prop.Client, prop.PieceCID, prop.Label)
							}
						}
					}
				}
			}
		}

		return nil
	},
}
