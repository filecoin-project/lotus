package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

var infoStateGrowthSimCommand = &cli.Command{
	Name:        "state-size",
	Description: "List daily state size over the course of the simulation starting at the end.",
	Action: func(cctx *cli.Context) (err error) {
		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := node.Close(); err == nil {
				err = cerr
			}
		}()

		sim, err := node.LoadSim(cctx.Context, cctx.String("simulation"))
		if err != nil {
			return err
		}

		// NOTE: This code is entirely read-bound.
		store := node.Chainstore.StateBlockstore()
		stateSize := func(ctx context.Context, c cid.Cid) (uint64, error) {
			seen := cid.NewSet()
			sema := make(chan struct{}, 40)
			var lock sync.Mutex
			var recSize func(cid.Cid) (uint64, error)
			recSize = func(c cid.Cid) (uint64, error) {
				// Not a part of the chain state.
				if err := ctx.Err(); err != nil {
					return 0, err
				}

				lock.Lock()
				visit := seen.Visit(c)
				lock.Unlock()
				// Already seen?
				if !visit {
					return 0, nil
				}

				var links []cid.Cid
				var totalSize uint64
				if err := store.View(cctx.Context, c, func(data []byte) error {
					totalSize += uint64(len(data))
					return cbg.ScanForLinks(bytes.NewReader(data), func(c cid.Cid) {
						if c.Prefix().Codec != cid.DagCBOR {
							return
						}

						links = append(links, c)
					})
				}); err != nil {
					return 0, err
				}

				var wg sync.WaitGroup
				errCh := make(chan error, 1)
				cb := func(c cid.Cid) {
					size, err := recSize(c)
					if err != nil {
						select {
						case errCh <- err:
						default:
						}
						return
					}
					atomic.AddUint64(&totalSize, size)
				}
				asyncCb := func(c cid.Cid) {
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer func() { <-sema }()
						cb(c)
					}()
				}
				for _, link := range links {
					select {
					case sema <- struct{}{}:
						asyncCb(link)
					default:
						cb(link)
					}

				}
				wg.Wait()

				select {
				case err := <-errCh:
					return 0, err
				default:
				}

				return totalSize, nil
			}
			return recSize(c)
		}

		firstEpoch := sim.GetStart().Height()
		ts := sim.GetHead()
		lastHeight := abi.ChainEpoch(math.MaxInt64)
		for ts.Height() > firstEpoch && cctx.Err() == nil {
			if ts.Height()+builtin.EpochsInDay <= lastHeight {
				lastHeight = ts.Height()

				parentStateSize, err := stateSize(cctx.Context, ts.ParentState())
				if err != nil {
					return err
				}

				_, _ = fmt.Fprintf(cctx.App.Writer, "%d: %s\n", ts.Height(), types.SizeStr(types.NewInt(parentStateSize)))
			}

			ts, err = sim.Node.Chainstore.LoadTipSet(cctx.Context, ts.Parents())
			if err != nil {
				return err
			}
		}
		return cctx.Err()
	},
}
