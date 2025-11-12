package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	miner15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	minertypes "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/must"
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
			totalProvenSectorsFee := big.NewInt(0)
			totalFeeDeduction := big.NewInt(0)

			// Epoch at which sectors that are within this deadline would have last been proven, so we
			// can look for new sectors that have (probably) not been proven yet
			dlLastProvenEpoch := dlinfo.PeriodStart + abi.ChainEpoch(dlIdx+1)*dlinfo.WPoStChallengeWindow
			if dlLastProvenEpoch > dlinfo.CurrentEpoch {
				dlLastProvenEpoch -= dlinfo.WPoStProvingPeriod
			}

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
						if soci16.Activation < dlLastProvenEpoch {
							totalProvenSectorsFee = big.Add(totalProvenSectorsFee, soci16.DailyFee)
						} // else the sector is too new, hasn't been proven and won't have paid a fee yet
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
			_, _ = fmt.Fprintf(cctx.App.Writer, "\t%s Deadline daily fee: %s, power: %s (proven sector fee sum: %s, total sector fee sum: %s, expiration fee deduction sum: %s)\n", correct, dl.DailyFee, dl.LivePower.QA, totalProvenSectorsFee, totalSectorsFee, totalFeeDeduction)

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
					// StateCompute at the height we arrived at, which may not be dlEndEpoch if nulls were involved
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

							// Implement top-down filtering based on expected cron execution flow
							switch depth {
							case 0:
								// At depth 0: SystemActorAddr (f00) -> CronActorAddr (f03) method 2 (EpochTick)
								if !(trace.Msg.From == builtin.SystemActorAddr && trace.Msg.To == builtin.CronActorAddr && trace.Msg.Method == 2) {
									return "", false, nil
								}
							case 1:
								// At depth 1: CronActorAddr (f03) -> StoragePowerActorAddr (f04) method 5 (OnEpochTickEnd)
								if !(trace.Msg.From == builtin.CronActorAddr && trace.Msg.To == power.Address && trace.Msg.Method == 5) {
									return "", false, nil
								}
							case 2:
								// At depth 2: StoragePowerActorAddr (f04) -> miner actors method 12 (OnDeferredCronEvent)
								if !(trace.Msg.From == power.Address && trace.Msg.Method == 12) {
									return "", false, nil
								}
							}

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

					expectedFee := totalProvenSectorsFee

					rewardPercent := "<cron call not found>"
					feeCapDesc := "fee not capped"
					if cronParams != nil {
						rew := miner16.ExpectedRewardForPower(cronParams.RewardSmoothed, cronParams.QualityAdjPowerSmoothed, dl.LivePower.QA, builtin.EpochsInDay)
						// 50% daily reward is our cap on dl.DailyFee
						rewardPercent = fmt.Sprintf("%s%%", big.Div(big.Mul(rew, big.NewInt(100)), dl.DailyFee))
						feeCap := big.Div(rew, big.NewInt(miner16.DailyFeeBlockRewardCapDenom))
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

var minerFeesInspect = &cli.Command{
	Name:      "fees-inspect",
	UsageText: "lotus-shed miner fees-inspect [--tipset <start-epoch>] [--count <epochs>] [--output <file>]",
	Description: "Inspect miner fees starting from the given tipset and going forward. The output is a CSV with the following columns:\n" +
		"Epoch, Burn, Fees, Penalties, Expected, Miners ...\n" +
		"Where:\n" +
		"  - Epoch: the epoch of the tipset\n" +
		"  - Burn: the total amount of attoFIL burned by miners in this tipset\n" +
		"  - Fees: the total amount of expected proof fees paid by miners in this tipset\n" +
		"  - Penalties: the total amount of penalties expected to be paid by miners in this tipset\n" +
		"  - Expected: whether the sum of fees and penalties equals the burn amount (✓ or ✗)\n" +
		"    A discrepancy here likely results from burnt precommit deposits or miners who can't pay fees,\n" +
		"    neither of which are currently calculated by this tool\n" +
		"  - Miners: the list of miners that burned or were expected to burn in this tipset\n\n" +
		"Output Options:\n" +
		"  --output <file> : Write CSV to file and track inspection progress. If file exists, inspection will resume from last epoch.\n\n" +
		"Examples:\n" +
		"  # Inspect 100 epochs starting from epoch 5100000\n" +
		"  lotus-shed miner fees-inspect --tipset @5100000 --count 100\n\n" +
		"  # Inspect from epoch 5100000 to current head\n" +
		"  lotus-shed miner fees-inspect --tipset @5100000\n\n" +
		"  # Save results to file, inspect 1000 epochs from epoch 5000000\n" +
		"  lotus-shed miner fees-inspect --tipset @5000000 --count 1000 --output fees.csv\n\n" +
		"  # Resume from last epoch in existing file (no --tipset needed)\n" +
		"  lotus-shed miner fees-inspect --output fees.csv --count 100\n\n" +
		"  # Resume and go to current head\n" +
		"  lotus-shed miner fees-inspect --output fees.csv",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "starting tipset or height (@X to specify an epoch) - required unless resuming from file",
		},
		&cli.IntFlag{
			Name:  "count",
			Usage: "number of epochs to inspect forward from --tipset (default: to current head)",
		},
		&cli.StringFlag{
			Name:  "output",
			Usage: "CSV file to save or resume results",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		burnAddr, err := address.NewFromString("f099")
		if err != nil {
			return xerrors.Errorf("parsing burn address: %w", err)
		}

		bstore := blockstore.NewAPIBlockstore(api)
		adtStore := adt.WrapStore(ctx, cbor.NewCborStore(bstore))

		outputFile := cctx.String("output")
		count := cctx.Int("count")

		// Handle output options
		var csvWriter *csv.Writer
		var lastEpoch abi.ChainEpoch = 0

		if outputFile != "" {
			// Check if file exists to resume from it
			if _, err := os.Stat(outputFile); err == nil {
				// Read the last epoch from existing file
				f, err := os.Open(outputFile)
				if err != nil {
					return xerrors.Errorf("opening output file: %w", err)
				}
				reader := csv.NewReader(f)
				var lastLine []string
				for {
					line, err := reader.Read()
					if err == io.EOF {
						break
					}
					if err != nil {
						_ = f.Close()
						return xerrors.Errorf("reading output file: %w", err)
					}
					if line[0] != "Epoch" { // Skip header
						lastLine = line
					}
				}
				_ = f.Close()

				if len(lastLine) > 0 {
					epochStr := strings.TrimSpace(lastLine[0])
					epoch, err := strconv.ParseInt(epochStr, 10, 64)
					if err != nil {
						return xerrors.Errorf("parsing last epoch from output file: %w", err)
					}
					lastEpoch = abi.ChainEpoch(epoch)
					_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Last processed epoch: %d\n", lastEpoch)
				}
			}

			// Open file for appending or create new one
			var f *os.File
			if lastEpoch > 0 {
				f, err = os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					return xerrors.Errorf("opening output file for append: %w", err)
				}
			} else {
				f, err = os.Create(outputFile)
				if err != nil {
					return xerrors.Errorf("creating output file: %w", err)
				}
			}
			defer func() { _ = f.Close() }()

			csvWriter = csv.NewWriter(f)
			defer csvWriter.Flush()

			// Write header if new file
			if lastEpoch == 0 {
				if err := csvWriter.Write([]string{"Epoch", "Burn", "Fees", "Penalties", "Expected", "Miners"}); err != nil {
					return xerrors.Errorf("writing output header: %w", err)
				}
			}
		}

		inspectTipset := func(ts *types.TipSet) error {
			compute, err := api.StateCompute(ctx, ts.Height(), nil, ts.Key())
			if err != nil {
				return xerrors.Errorf("computing tipset at deadline end epoch: %w", err)
			}

			// We expect this to be non-nil when we need it because we only dig into details once we
			// know we are inside a cron call. We use the params from cron to avoid having to go and
			// fetch RewardSmoothed and QualityAdjPowerSmoothed for the current epoch from the reward
			// and power actors (but we could).
			var cronParams *miner16.DeferredCronEventParams

			type minerBurn struct {
				addr    address.Address
				burn    big.Int
				fee     big.Int
				penalty big.Int
			}

			// Calculate the expected penalty for a given power amount. This is unfortunately complicated
			// by the need to fetch total network reward and power for the current tipset.
			faultFeeForPower := func(qaPower abi.StoragePower) (abi.TokenAmount, error) {
				nv, err := api.StateNetworkVersion(ctx, ts.Key())
				if err != nil {
					return big.Zero(), err
				}

				return minertypes.PledgePenaltyForContinuedFault(
					nv,
					builtin.FilterEstimate{
						PositionEstimate: cronParams.RewardSmoothed.PositionEstimate,
						VelocityEstimate: cronParams.RewardSmoothed.VelocityEstimate,
					},
					builtin.FilterEstimate{
						PositionEstimate: cronParams.QualityAdjPowerSmoothed.PositionEstimate,
						VelocityEstimate: cronParams.QualityAdjPowerSmoothed.VelocityEstimate,
					},
					qaPower,
				)
			}

			// Inspect a miner actor for the current tipset and calculate the expected fee and penalty
			// amounts for the deadline it is in.
			inspectMiner := func(minerAddr address.Address) (big.Int, big.Int, error) {
				minerActor, err := api.StateGetActor(ctx, minerAddr, ts.Key())
				if err != nil {
					return big.Zero(), big.Zero(), xerrors.Errorf("getting miner actor: %w", err)
				}
				var minerState miner16.State
				if err := adtStore.Get(ctx, minerActor.Head, &minerState); err != nil {
					return big.Zero(), big.Zero(), xerrors.Errorf("getting miner state: %w", err)
				}

				dlinfo, err := api.StateMinerProvingDeadline(ctx, minerAddr, ts.Key())
				if err != nil {
					return big.Zero(), big.Zero(), xerrors.Errorf("getting miner proving deadline: %w", err)
				}

				deadlines, err := minerState.LoadDeadlines(adtStore)
				if err != nil {
					return big.Zero(), big.Zero(), xerrors.Errorf("loading deadlines: %w", err)
				}

				deadline, err := deadlines.LoadDeadline(adtStore, dlinfo.Index)
				if err != nil {
					return big.Zero(), big.Zero(), xerrors.Errorf("loading deadline: %w", err)
				}

				faultFee := big.Zero()
				if !deadline.FaultyPower.QA.IsZero() {
					faultFee, err = faultFeeForPower(deadline.FaultyPower.QA)
					if err != nil {
						return big.Zero(), big.Zero(), xerrors.Errorf("getting fault fee: %w", err)
					}
				}

				expectedFee := deadline.DailyFee
				// Check if the fees should be capped to 50% of expected daily reward; this is an unlikely
				// case and we could ignore it and still be correct almost all of the time.
				rew := miner16.ExpectedRewardForPower(cronParams.RewardSmoothed, cronParams.QualityAdjPowerSmoothed, deadline.LivePower.QA, builtin.EpochsInDay)
				feeCap := big.Div(rew, big.NewInt(miner16.DailyFeeBlockRewardCapDenom))
				if feeCap.LessThan(expectedFee) {
					expectedFee = feeCap
				}

				return expectedFee, faultFee, nil
			}

			// Dig into a call and its subcalls to find (1) cron calls to a miner, and subsequent to that
			// in the same trace, (2) miner calls to the burn address. We then have the total burn for
			// an individual miner so we proceed to collect the expected fee and penalty amounts for that
			// miner in their current deadline (which we expect we are processing the end of in this cron
			// call).
			burns := make([]*minerBurn, 0)
			cronMinerCalls := make(map[address.Address]struct{})
			var traceBurns func(depth int, trace types.ExecutionTrace, thisExecCronMiner *minerBurn) error
			traceBurns = func(depth int, trace types.ExecutionTrace, thisExecCronMiner *minerBurn) error {
				// Implement top-down filtering based on expected cron execution flow
				switch depth {
				case 0:
					// At depth 0: SystemActorAddr (f00) -> CronActorAddr (f03) method 2 (EpochTick)
					if !(trace.Msg.From == builtin.SystemActorAddr && trace.Msg.To == builtin.CronActorAddr && trace.Msg.Method == 2) {
						return nil
					}
				case 1:
					// At depth 1: CronActorAddr (f03) -> StoragePowerActorAddr (f04) method 5 (OnEpochTickEnd)
					if !(trace.Msg.From == builtin.CronActorAddr && trace.Msg.To == power.Address && trace.Msg.Method == 5) {
						return nil
					}
				case 2:
					// At depth 2: StoragePowerActorAddr (f04) -> miner actors method 12 (OnDeferredCronEvent)
					if !(trace.Msg.From == power.Address && trace.Msg.Method == 12) {
						return nil
					}
				}

				if trace.Msg.From == power.Address && trace.Msg.Method == 12 {
					// cron call to miner
					if thisExecCronMiner != nil {
						if _, ok := cronMinerCalls[thisExecCronMiner.addr]; ok {
							return xerrors.Errorf("multiple cron calls to same miner in one message: %s", *thisExecCronMiner)
						}
					}

					var p miner16.DeferredCronEventParams
					if err := p.UnmarshalCBOR(bytes.NewReader(trace.Msg.Params)); err != nil {
						return xerrors.Errorf("unmarshalling cron params: %w", err)
					}
					cronParams = &p

					thisExecCronMiner = &minerBurn{
						addr:    trace.Msg.To,
						burn:    big.Zero(),
						fee:     big.Zero(),
						penalty: big.Zero(),
					}
					burns = append(burns, thisExecCronMiner)
					cronMinerCalls[trace.Msg.To] = struct{}{}
				} else if thisExecCronMiner != nil && trace.Msg.From == thisExecCronMiner.addr && trace.Msg.To == burnAddr {
					// TODO: handle multiple burn? Shouldn't happen but maybe it should be checked?
					thisExecCronMiner.burn = trace.Msg.Value

					// If we have a burn, we can inspect the miner for fees and penalties.
					if thisExecCronMiner.burn.IsZero() {
						return nil
					}

					fee, penalty, err := inspectMiner(trace.Msg.From)
					if err != nil {
						return xerrors.Errorf("inspecting miner: %w", err)
					}

					thisExecCronMiner.fee = fee
					thisExecCronMiner.penalty = penalty
				}

				for _, st := range trace.Subcalls {
					if err := traceBurns(depth+1, st, thisExecCronMiner); err != nil {
						return err
					}
				}

				return nil
			}

			// For each execution in this tipset, find the ones that are cron calls to miners and have
			// burn.
			for _, invoc := range compute.Trace {
				if err := traceBurns(0, invoc.ExecutionTrace, nil); err != nil {
					return xerrors.Errorf("printing execution trace: %w", err)
				}
			}

			totalBurn := big.Zero()
			totalFees := big.Zero()
			totalPenalties := big.Zero()
			miners := make([]address.Address, 0)
			for _, b := range burns {
				totalBurn = big.Add(totalBurn, b.burn)
				totalFees = big.Add(totalFees, b.fee)
				totalPenalties = big.Add(totalPenalties, b.penalty)
				if !b.burn.IsZero() || !b.fee.IsZero() || !b.penalty.IsZero() {
					miners = append(miners, b.addr)
				}
			}
			expected := "✓"
			if !big.Add(totalFees, totalPenalties).Equals(totalBurn) {
				// This is likely because we are not including the precommit deposits that are burnt or
				// checking miner balances to see if they can pay or not. For correctness we could try and
				// calculate that for each miner.
				expected = "✗"
			}
			// Format output - unified CSV handling for both file and stdout
			row := []string{
				fmt.Sprintf("%d", ts.Height()),
				totalBurn.String(),
				totalFees.String(),
				totalPenalties.String(),
				expected,
			}

			// Add miners - consistent ordering
			sort.Slice(miners, func(i, j int) bool {
				return must.One(address.IDFromAddress(miners[i])) < must.One(address.IDFromAddress(miners[j]))
			})
			minersStr := ""
			for i, maddr := range miners {
				if i > 0 {
					minersStr += " "
				}
				minersStr += maddr.String()
			}
			row = append(row, minersStr)

			if err := csvWriter.Write(row); err != nil {
				return xerrors.Errorf("writing output: %w", err)
			}
			csvWriter.Flush()

			return nil
		}

		// Determine starting point
		var startTs *types.TipSet
		if lastEpoch > 0 && outputFile != "" {
			// Resume from last epoch in file
			// Check if we're already at the head
			head, err := api.ChainHead(ctx)
			if err != nil {
				return xerrors.Errorf("getting chain head: %w", err)
			}

			if lastEpoch >= head.Height() {
				_, _ = fmt.Fprintf(cctx.App.ErrWriter, "File is up to date (last epoch %d, current head %d)\n", lastEpoch, head.Height())
				return nil
			}

			startTs, err = api.ChainGetTipSetByHeight(ctx, lastEpoch+1, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting tipset at height %d: %w", lastEpoch+1, err)
			}
			_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Resuming from epoch %d\n", lastEpoch+1)
		} else {
			// Start from specified tipset
			if !cctx.IsSet("tipset") {
				return xerrors.Errorf("--tipset is required when not resuming from an existing file")
			}
			startTs, err = lcli.LoadTipSet(ctx, cctx, api)
			if err != nil {
				return err
			}
		}

		// Determine end point
		var endHeight abi.ChainEpoch
		if count > 0 {
			// Use specified count
			endHeight = startTs.Height() + abi.ChainEpoch(count) - 1
		} else {
			// Go to current head
			head, err := api.ChainHead(ctx)
			if err != nil {
				return xerrors.Errorf("getting chain head: %w", err)
			}
			endHeight = head.Height()
		}

		// Calculate actual count for the loop
		actualCount := int(endHeight - startTs.Height() + 1)
		if actualCount <= 0 {
			return xerrors.Errorf("invalid range: start epoch %d >= end epoch %d", startTs.Height(), endHeight)
		}

		// Setup CSV writer for stdout if no output file
		if outputFile == "" {
			csvWriter = csv.NewWriter(cctx.App.Writer)
			defer csvWriter.Flush()
			// Write header for stdout
			if err := csvWriter.Write([]string{"Epoch", "Burn", "Fees", "Penalties", "Expected", "Miners"}); err != nil {
				return xerrors.Errorf("writing output header: %w", err)
			}
		}

		// Process epochs forward from start to end
		currentTs := startTs

		// Print starting info if outputting to file
		if outputFile != "" {
			_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Processing %d epochs from %d to %d\n", actualCount, startTs.Height(), endHeight)
		}

		for i := 0; i < actualCount; i++ {
			// Show progress if outputting to file
			if outputFile != "" {
				_, _ = fmt.Fprintf(cctx.App.ErrWriter, "\rProcessing epoch %d [%d/%d] (%d%%)...", currentTs.Height(), i+1, actualCount, (i+1)*100/actualCount)
			}

			if err := inspectTipset(currentTs); err != nil {
				return xerrors.Errorf("inspecting tipset %d: %w", currentTs.Height(), err)
			}

			// Move to next epoch
			if i < actualCount-1 {
				nextHeight := currentTs.Height() + 1
				currentTs, err = api.ChainGetTipSetByHeight(ctx, nextHeight, types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("getting tipset at height %d: %w", nextHeight, err)
				}
			}
		}

		// Clear progress line if outputting to file
		if outputFile != "" {
			_, _ = fmt.Fprintf(cctx.App.ErrWriter, "\r                                                                       \r")
			_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Completed processing %d epochs\n", actualCount)
		}
		return nil
	},
}

var minerExpectedRewardCmd = &cli.Command{
	Name: "expected-reward",
	Usage: "Calculate the expected block reward for a miner over a specified projection period " +
		"(e.g. lotus-shed miner expected-reward --tipset @head --projection-period 1025280 --qapower 69793218560)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "tipset or height (@X or @head for latest)",
			Value: "@head",
		},
		&cli.Int64Flag{
			Name:  "projection-period",
			Usage: "number of epochs to project reward for",
			Value: builtin.EpochsInDay,
		},
		&cli.Int64Flag{
			Name:  "qapower",
			Usage: "Quality Adjusted Power in bytes",
			Value: 32 * 1024 * 1024 * 1024, // 32 GiB
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)
		bstore := blockstore.NewAPIBlockstore(api)
		adtStore := adt.WrapStore(ctx, cbor.NewCborStore(bstore))

		ts, err := lcli.LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		projectionPeriod := abi.ChainEpoch(cctx.Int64("projection-period"))
		qapower := abi.NewStoragePower(cctx.Int64("qapower"))

		// Get the reward value for the current tipset from the reward actor
		var rewardSmoothed builtin.FilterEstimate
		if act, err := api.StateGetActor(ctx, reward.Address, ts.Key()); err != nil {
			return xerrors.Errorf("loading reward actor: %w", err)
		} else if s, err := reward.Load(adtStore, act); err != nil {
			return xerrors.Errorf("loading reward actor state: %w", err)
		} else if rewardSmoothed, err = s.ThisEpochRewardSmoothed(); err != nil {
			return xerrors.Errorf("failed to determine smoothed reward: %w", err)
		}

		// Get the network power value for the current tipset from the power actor
		var powerSmoothed builtin.FilterEstimate
		if act, err := api.StateGetActor(ctx, power.Address, ts.Key()); err != nil {
			return xerrors.Errorf("loading power actor: %w", err)
		} else if s, err := power.Load(adtStore, act); err != nil {
			return xerrors.Errorf("loading power actor state: %w", err)
		} else if powerSmoothed, err = s.TotalPowerSmoothed(); err != nil {
			return xerrors.Errorf("failed to determine total power: %w", err)
		}

		nv, err := api.StateNetworkVersion(ctx, ts.Key())
		if err != nil {
			return xerrors.Errorf("getting network version: %w", err)
		}

		rew, err := minertypes.ExpectedRewardForPower(nv, rewardSmoothed, powerSmoothed, qapower, projectionPeriod)
		if err != nil {
			return xerrors.Errorf("calculating expected reward: %w", err)
		}
		_, _ = fmt.Fprintf(
			cctx.App.Writer,
			"Expected reward for %s bytes of QA power @ epoch %d for %d epochs: %s attoFIL\n",
			qapower,
			ts.Height(),
			projectionPeriod,
			rew,
		)
		return nil
	},
}
