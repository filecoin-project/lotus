package main

import (
	"bytes"
	"fmt"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	miner15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	"github.com/filecoin-project/specs-actors/v7/actors/builtin"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var minerFeesCmd = &cli.Command{
	Name:  "fees",
	Usage: "[miner address] [--tipset <tipset>]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "tipset or height (@X or @head for latest)",
			Value: "@head",
		},
		&cli.IntFlag{
			Name:  "proving-periods",
			Usage: "number of proving periods to check",
			Value: 1,
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
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

		burnAddr, err := address.NewFromString("f099")
		if err != nil {
			return xerrors.Errorf("parsing burn address: %w", err)
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

		dlinfo, err := api.StateMinerProvingDeadline(ctx, minerAddr, ts.Key())
		if err != nil {
			return xerrors.Errorf("getting miner proving deadline: %w", err)
		}

		var discrepancies bool
		totalMinerFee := big.NewInt(0)

		if err = deadlines.ForEach(adtStore, func(dlIdx uint64, dl *miner16.Deadline) error {
			_, _ = fmt.Fprintf(cctx.App.Writer, "Deadline %d:\n", dlIdx)

			totalSectorsFee := big.NewInt(0)
			totalFeeDeduction := big.NewInt(0)

			/** Iterate over partitions **/

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

				/** Iterate over expiration queue within each partition **/

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
			_, _ = fmt.Fprintf(cctx.App.Writer, "\t%s Deadline daily fee: %s, power: %s (sector fee sum: %s, expiration fee deduction sum: %s)\n", correct, dl.DailyFee, dl.LivePower.QA, totalSectorsFee, totalFeeDeduction)

			if !dl.DailyFee.IsZero() {

				// run back through the last proving-period days
				for day := int64(0); day < cctx.Int64("proving-periods"); day++ {

					/** Look for evidence of fee payment **/

					totalMinerFee = big.Add(totalMinerFee, dl.DailyFee)

					// Epoch at which the deadline ends and we expect to find cron execution
					dlEndEpoch := dlinfo.PeriodStart + abi.ChainEpoch(dlIdx+1)*dlinfo.WPoStChallengeWindow - 1 - miner16.WPoStProvingPeriod*abi.ChainEpoch(day)
					if dlEndEpoch > dlinfo.CurrentEpoch {
						dlEndEpoch -= dlinfo.WPoStProvingPeriod
					}
					// If there is a null epoch involved we don't want to go backward, so we go AfterHeight
					dlEndTs, err := api.ChainGetTipSetAfterHeight(ctx, dlEndEpoch, types.EmptyTSK)
					if err != nil {
						return xerrors.Errorf("getting tipset at deadline end epoch: %w", err)
					}
					// StateCompute at the height we arived at, which may not be dlEndEpoch if nulls were involved
					compute, err := api.StateCompute(ctx, dlEndTs.Height(), nil, dlEndTs.Key())
					if err != nil {
						return xerrors.Errorf("computing tipset at deadline end epoch: %w", err)
					}

					_, _ = fmt.Fprintf(cctx.App.Writer, "\tInspecting last deadline cron @ %d:\n", dlEndTs.Height())

					var burnValue *big.Int
					var cronParams *miner16.DeferredCronEventParams

					// Sort through traces to find relevant ones
					for _, invoc := range compute.Trace {
						var printExec func(depth int, trace types.ExecutionTrace) (string, bool, error)
						printExec = func(depth int, trace types.ExecutionTrace) (string, bool, error) {
							ec := ""
							if trace.MsgRct.ExitCode != 0 {
								ec = fmt.Sprintf("(!%d)", trace.MsgRct.ExitCode)
							}
							params := ""
							var has bool

							if trace.Msg.From == minerAddr || trace.Msg.To == minerAddr {
								has = true // involves our miner

								if trace.Msg.From == power.Address && trace.Msg.Method == 12 {
									// cron call to miner
									if cronParams != nil {
										return "", false, xerrors.New("multiple cron calls to miner in one message")
									}
									var p miner16.DeferredCronEventParams
									if err := p.UnmarshalCBOR(bytes.NewReader(trace.Msg.Params)); err != nil {
										return "", false, xerrors.Errorf("unmarshalling cron params: %w", err)
									}
									cronParams = &p
								} else if trace.Msg.From == minerAddr && trace.Msg.To == burnAddr {
									// miner paying fee debt
									if burnValue != nil {
										return "", false, xerrors.New("unexpected multiple miner burns in one message")
									}
									burnValue = &trace.Msg.Value
								}
							}

							s := fmt.Sprintf(" %s->%s[%d]:%v%s%s", trace.Msg.From, trace.Msg.To, trace.Msg.Method, trace.Msg.Value, ec, params)

							for _, st := range trace.Subcalls {
								if ss, shas, err := printExec(depth+1, st); err != nil {
									return "", false, err
								} else {
									if !has && shas {
										has = true
									}
									s += ss
								}
							}

							if depth == 0 {
								s += ";"
							}

							return s, has, nil
						}
						if pr, has, err := printExec(0, invoc.ExecutionTrace); err != nil {
							return xerrors.Errorf("printing execution trace: %w", err)
						} else if has {
							_, _ = fmt.Fprintf(cctx.App.Writer, "\t\tTrace involving %s:%s\n", minerAddr, pr)
						}
					}

					expectedFee := dl.DailyFee

					rewardPercent := "<cron call not found>"
					feeCapDesc := "fee not capped"
					if cronParams != nil {
						rew := miner16.ExpectedRewardForPower(cronParams.RewardSmoothed, cronParams.QualityAdjPowerSmoothed, dl.LivePower.QA, builtin.EpochsInDay)
						// 50% daily reward is our cap on dl.DailyFee
						rewardPercent = fmt.Sprintf("%s%%", big.Div(big.Mul(rew, big.NewInt(100)), dl.DailyFee))
						feeCap := big.Div(rew, big.NewInt(2))
						if feeCap.LessThan(dl.DailyFee) {
							feeCapDesc = fmt.Sprintf("fee capped @ %s", feeCap)
							expectedFee = feeCap
						}
					}
					_, _ = fmt.Fprintf(cctx.App.Writer, "\t\tEstimated deadline daily reward: %s of fee (%s)\n", rewardPercent, feeCapDesc)

					burnValueStr := "<burn not found>"
					burnStatus := "✗"
					burnExtra := ""
					if burnValue != nil {
						burnValueStr = burnValue.String()
						if burnValue.Equals(expectedFee) {
							burnStatus = "✓"
						} else if burnValue.LessThan(expectedFee) {
							discrepancies = true
							burnExtra = " (discrepancy found, burn too low)"
						} else {
							burnExtra = " (higher burn than expected, possible fault fee)"
						}
					} else {
						if expectedFee.IsZero() {
							burnStatus = "✓" // we don't expect to find a burn in this special case
						} else {
							discrepancies = true
						}
					}
					_, _ = fmt.Fprintf(cctx.App.Writer, "\t\t%s Burn value: %s%s\n", burnStatus, burnValueStr, burnExtra)

					/** Print various balances **/

					actBefore, err := api.StateGetActor(ctx, minerAddr, dlEndTs.Key())
					if err != nil {
						return xerrors.Errorf("getting miner actor before: %w", err)
					}
					var stateBefore miner16.State
					if err := adtStore.Get(ctx, actBefore.Head, &stateBefore); err != nil {
						return xerrors.Errorf("getting miner state: %w", err)
					}
					dlNextTs, err := api.ChainGetTipSetAfterHeight(ctx, dlEndTs.Height()+1, types.EmptyTSK)
					if err != nil {
						return xerrors.Errorf("getting tipset after deadline end: %w", err)
					}
					actAfter, err := api.StateGetActor(ctx, minerAddr, dlNextTs.Key())
					if err != nil {
						return xerrors.Errorf("getting miner actor before: %w", err)
					}
					var stateAfter miner16.State
					if err := adtStore.Get(ctx, actAfter.Head, &stateAfter); err != nil {
						return xerrors.Errorf("getting miner state: %w", err)
					}

					balanceDelta := big.Sub(actAfter.Balance, actBefore.Balance)
					balanceStatus := "✓"
					if !balanceDelta.Neg().Equals(expectedFee) {
						// We found a balance change, but it doesn't match the fee
						balanceStatus = "✗"
						discrepancies = true
					}

					_, _ = fmt.Fprintf(
						cctx.App.Writer,
						"\t\t%s Balance Δ: %s, LockedFunds Δ: %s, FeeDebt: %s, \n",
						balanceStatus,
						balanceDelta,
						big.Sub(stateAfter.LockedFunds, stateBefore.LockedFunds),
						stateAfter.FeeDebt,
					)
				}
			}
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
