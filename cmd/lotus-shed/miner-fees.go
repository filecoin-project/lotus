package main

import (
	"bytes"
	"fmt"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	miner15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var minerFeesCmd = &cli.Command{
	Name:  "miner-fees",
	Usage: "[miner address] [--tipset <tipset>]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "tipset or height (@X or @head for latest)",
			Value: "@head",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("must provide miner address")
		}
		minerAddr, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("parsing miner address: %w", err)
		}

		ts, err := lcli.LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		bstore := blockstore.NewAPIBlockstore(api)
		adtStore := adt.WrapStore(ctx, cbor.NewCborStore(bstore))

		minerActor, err := api.StateGetActor(ctx, minerAddr, ts.Key())
		if err != nil {
			return xerrors.Errorf("getting miner actor: %w", err)
		}
		var minerState miner16.State
		if err := adtStore.Get(ctx, minerActor.Head, &minerState); err != nil {
			return xerrors.Errorf("getting miner state: %w", err)
		}
		sectors, err := miner16.LoadSectors(adtStore, minerState.Sectors)
		if err != nil {
			return xerrors.Errorf("loading sectors: %w", err)
		}

		deadlines, err := minerState.LoadDeadlines(adtStore)
		if err != nil {
			return xerrors.Errorf("loading deadlines: %w", err)
		}

		var discrepancies bool
		totalMinerFee := big.NewInt(0)

		if err = deadlines.ForEach(adtStore, func(dlIdx uint64, dl *miner16.Deadline) error {
			_, _ = fmt.Fprintf(cctx.App.Writer, "Deadline %d:\n", dlIdx)

			totalSectorsFee := big.NewInt(0)
			totalFeeDeduction := big.NewInt(0)

			partitions, err := dl.PartitionsArray(adtStore)
			if err != nil {
				return xerrors.Errorf("loading partitions: %w", err)
			}
			var partition miner16.Partition
			if err = partitions.ForEach(&partition, func(i int64) error {
				liveSectors, err := partition.LiveSectors()
				if err != nil {
					return xerrors.Errorf("loading live sectors: %w", err)
				}
				if err = liveSectors.ForEach(func(u uint64) error {
					var val cbg.Deferred
					if has, err := sectors.Array.Get(u, &val); err != nil {
						return xerrors.Errorf("getting sector %d: %w", u, err)
					} else if !has {
						return xerrors.Errorf("sector %d not found", u)
					}

					_, _ = fmt.Fprintf(cctx.App.Writer, "\tSector %d daily fee: ", u)
					// if we can load it as v15, it's not migrated and has no fee
					var soci15 miner15.SectorOnChainInfo
					if err := soci15.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
						// if we can't load it as v15, try v16
						var soci16 miner16.SectorOnChainInfo
						if err := soci16.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
							return err
						}
						if soci16.DailyFee.IsZero() {
							_, _ = fmt.Fprintf(cctx.App.Writer, "<legacy>\n")
						} else {
							_, _ = fmt.Fprintf(cctx.App.Writer, "%s\n", soci16.DailyFee)
						}
						totalSectorsFee = big.Add(totalSectorsFee, soci16.DailyFee)
					} else {
						_, _ = fmt.Fprintf(cctx.App.Writer, "<legacy, not migrated>\n")
					}
					return nil
				}); err != nil {
					return xerrors.Errorf("iterating sectors: %w", err)
				}

				if expQ, err := miner16.LoadExpirationQueue(adtStore, partition.ExpirationsEpochs, minerState.QuantSpecForDeadline(dlIdx), miner16.PartitionExpirationAmtBitwidth); err != nil {
					return xerrors.Errorf("loading expiration queue: %w", err)
				} else {
					var exp miner16.ExpirationSet
					if err = expQ.ForEach(&exp, func(e int64) error {
						totalFeeDeduction = big.Add(totalFeeDeduction, exp.FeeDeduction)
						return nil
					}); err != nil {
						return xerrors.Errorf("iterating expirations: %w", err)
					}
				}

				if err != nil {
					return xerrors.Errorf("iterating expirations: %w", err)
				}
				return nil
			}); err != nil {
				return xerrors.Errorf("iterating partitions: %w", err)
			}

			correct := "✓"
			if !dl.DailyFee.Equals(totalSectorsFee) || !dl.DailyFee.Equals(totalFeeDeduction) {
				correct = "✗"
				discrepancies = true
			}
			_, _ = fmt.Fprintf(cctx.App.Writer, "\t%s Deadline daily fee: %s (sector fee sum: %s, expiration fee deduction sum: %s)\n", correct, dl.DailyFee, totalSectorsFee, totalFeeDeduction)

			totalMinerFee = big.Add(totalMinerFee, dl.DailyFee)
			return nil
		}); err != nil {
			return xerrors.Errorf("iterating deadlines: %w", err)
		}

		_, _ = fmt.Fprintf(cctx.App.Writer, "Total miner daily fee: %s / %s (", totalMinerFee, types.FIL(totalMinerFee))
		if discrepancies {
			_, _ = fmt.Fprintf(cctx.App.Writer, "✗ discrepancies found!)\n")
		} else {
			_, _ = fmt.Fprintf(cctx.App.Writer, "✓ no discrepancies found)\n")
		}

		return nil
	},
}
