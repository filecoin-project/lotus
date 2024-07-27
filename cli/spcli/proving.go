package spcli

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

func ProvingInfoCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "View current state information",
		Action: func(cctx *cli.Context) error {
			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			maddr, err := getActorAddress(cctx)
			if err != nil {
				return err
			}

			head, err := api.ChainHead(ctx)
			if err != nil {
				return xerrors.Errorf("getting chain head: %w", err)
			}

			mact, err := api.StateGetActor(ctx, maddr, head.Key())
			if err != nil {
				return err
			}

			stor := store.ActorStore(ctx, blockstore.NewAPIBlockstore(api))

			mas, err := miner.Load(stor, mact)
			if err != nil {
				return err
			}

			cd, err := api.StateMinerProvingDeadline(ctx, maddr, head.Key())
			if err != nil {
				return xerrors.Errorf("getting miner info: %w", err)
			}

			fmt.Printf("Miner: %s\n", color.BlueString("%s", maddr))

			proving := uint64(0)
			faults := uint64(0)
			recovering := uint64(0)
			curDeadlineSectors := uint64(0)

			if err := mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
				return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
					if bf, err := part.LiveSectors(); err != nil {
						return err
					} else if count, err := bf.Count(); err != nil {
						return err
					} else {
						proving += count
						if dlIdx == cd.Index {
							curDeadlineSectors += count
						}
					}

					if bf, err := part.FaultySectors(); err != nil {
						return err
					} else if count, err := bf.Count(); err != nil {
						return err
					} else {
						faults += count
					}

					if bf, err := part.RecoveringSectors(); err != nil {
						return err
					} else if count, err := bf.Count(); err != nil {
						return err
					} else {
						recovering += count
					}

					return nil
				})
			}); err != nil {
				return xerrors.Errorf("walking miner deadlines and partitions: %w", err)
			}

			var faultPerc float64
			if proving > 0 {
				faultPerc = float64(faults * 100 / proving)
			}

			fmt.Printf("Current Epoch:           %d\n", cd.CurrentEpoch)

			fmt.Printf("Proving Period Boundary: %d\n", cd.PeriodStart%cd.WPoStProvingPeriod)
			fmt.Printf("Proving Period Start:    %s\n", cliutil.EpochTimeTs(cd.CurrentEpoch, cd.PeriodStart, head))
			fmt.Printf("Next Period Start:       %s\n\n", cliutil.EpochTimeTs(cd.CurrentEpoch, cd.PeriodStart+cd.WPoStProvingPeriod, head))

			fmt.Printf("Faults:      %d (%.2f%%)\n", faults, faultPerc)
			fmt.Printf("Recovering:  %d\n", recovering)

			fmt.Printf("Deadline Index:       %d\n", cd.Index)
			fmt.Printf("Deadline Sectors:     %d\n", curDeadlineSectors)
			fmt.Printf("Deadline Open:        %s\n", cliutil.EpochTime(cd.CurrentEpoch, cd.Open))
			fmt.Printf("Deadline Close:       %s\n", cliutil.EpochTime(cd.CurrentEpoch, cd.Close))
			fmt.Printf("Deadline Challenge:   %s\n", cliutil.EpochTime(cd.CurrentEpoch, cd.Challenge))
			fmt.Printf("Deadline FaultCutoff: %s\n", cliutil.EpochTime(cd.CurrentEpoch, cd.FaultCutoff))
			return nil
		},
	}
}

func ProvingDeadlinesCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:  "deadlines",
		Usage: "View the current proving period deadlines information",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "all",
				Usage:   "Count all sectors (only live sectors are counted by default)",
				Aliases: []string{"a"},
			},
		},
		Action: func(cctx *cli.Context) error {
			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			maddr, err := getActorAddress(cctx)
			if err != nil {
				return err
			}

			deadlines, err := api.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting deadlines: %w", err)
			}

			di, err := api.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting deadlines: %w", err)
			}

			head, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}

			fmt.Printf("Miner: %s\n", color.BlueString("%s", maddr))

			tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "deadline\topen\tpartitions\tsectors (faults)\tproven partitions")

			for dlIdx, deadline := range deadlines {
				partitions, err := api.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("getting partitions for deadline %d: %w", dlIdx, err)
				}

				provenPartitions, err := deadline.PostSubmissions.Count()
				if err != nil {
					return err
				}

				sectors := uint64(0)
				faults := uint64(0)
				var partitionCount int

				for _, partition := range partitions {
					if !cctx.Bool("all") {
						sc, err := partition.LiveSectors.Count()
						if err != nil {
							return err
						}

						if sc > 0 {
							partitionCount++
						}

						sectors += sc
					} else {
						sc, err := partition.AllSectors.Count()
						if err != nil {
							return err
						}

						partitionCount++
						sectors += sc
					}

					fc, err := partition.FaultySectors.Count()
					if err != nil {
						return err
					}

					faults += fc
				}

				var cur string
				if di.Index == uint64(dlIdx) {
					cur += "\t(current)"
				}

				_, _ = fmt.Fprintf(tw, "%d\t%s\t%d\t%d (%d)\t%d%s\n", dlIdx, deadlineOpenTime(head, uint64(dlIdx), di),
					partitionCount, sectors, faults, provenPartitions, cur)
			}

			return tw.Flush()
		},
	}
}

func deadlineOpenTime(ts *types.TipSet, dlIdx uint64, di *dline.Info) string {
	gapIdx := dlIdx - di.Index
	gapHeight := uint64(di.WPoStProvingPeriod) / di.WPoStPeriodDeadlines * gapIdx

	openHeight := di.Open + abi.ChainEpoch(gapHeight)
	genesisBlockTimestamp := ts.MinTimestamp() - uint64(ts.Height())*buildconstants.BlockDelaySecs

	return time.Unix(int64(genesisBlockTimestamp+buildconstants.BlockDelaySecs*uint64(openHeight)), 0).Format(time.TimeOnly)
}

func ProvingDeadlineInfoCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:  "deadline",
		Usage: "View the current proving period deadline information by its index",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "sector-nums",
				Aliases: []string{"n"},
				Usage:   "Print sector/fault numbers belonging to this deadline",
			},
			&cli.BoolFlag{
				Name:    "bitfield",
				Aliases: []string{"b"},
				Usage:   "Print partition bitfield stats",
			},
		},
		ArgsUsage: "<deadlineIdx>",
		Action: func(cctx *cli.Context) error {

			if cctx.NArg() != 1 {
				return lcli.IncorrectNumArgs(cctx)
			}

			dlIdx, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
			if err != nil {
				return xerrors.Errorf("could not parse deadline index: %w", err)
			}

			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			maddr, err := getActorAddress(cctx)
			if err != nil {
				return err
			}

			deadlines, err := api.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting deadlines: %w", err)
			}

			di, err := api.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting deadlines: %w", err)
			}

			partitions, err := api.StateMinerPartitions(ctx, maddr, dlIdx, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting partitions for deadline %d: %w", dlIdx, err)
			}

			head, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}

			provenPartitions, err := deadlines[dlIdx].PostSubmissions.Count()
			if err != nil {
				return err
			}

			fmt.Printf("Deadline Index:           %d\n", dlIdx)
			fmt.Printf("Deadline Open:            %s\n", deadlineOpenTime(head, dlIdx, di))
			fmt.Printf("Partitions:               %d\n", len(partitions))
			fmt.Printf("Proven Partitions:        %d\n", provenPartitions)
			fmt.Printf("Current:                  %t\n\n", di.Index == dlIdx)

			for pIdx, partition := range partitions {
				fmt.Printf("Partition Index:          %d\n", pIdx)

				printStats := func(bf bitfield.BitField, name string) error {
					count, err := bf.Count()
					if err != nil {
						return err
					}

					rit, err := bf.RunIterator()
					if err != nil {
						return err
					}

					if cctx.Bool("bitfield") {
						var ones, zeros, oneRuns, zeroRuns, invalid uint64
						for rit.HasNext() {
							r, err := rit.NextRun()
							if err != nil {
								return xerrors.Errorf("next run: %w", err)
							}
							if !r.Valid() {
								invalid++
							}
							if r.Val {
								ones += r.Len
								oneRuns++
							} else {
								zeros += r.Len
								zeroRuns++
							}
						}

						var buf bytes.Buffer
						if err := bf.MarshalCBOR(&buf); err != nil {
							return err
						}
						sz := len(buf.Bytes())
						szstr := types.SizeStr(types.NewInt(uint64(sz)))

						fmt.Printf("\t%s Sectors:%s%d (bitfield - runs %d+%d=%d - %d 0s %d 1s - %d inv - %s %dB)\n", name, strings.Repeat(" ", 18-len(name)), count, zeroRuns, oneRuns, zeroRuns+oneRuns, zeros, ones, invalid, szstr, sz)
					} else {
						fmt.Printf("\t%s Sectors:%s%d\n", name, strings.Repeat(" ", 18-len(name)), count)
					}

					if cctx.Bool("sector-nums") {
						nums, err := bf.All(count)
						if err != nil {
							return err
						}
						fmt.Printf("\t%s Sector Numbers:%s%v\n", name, strings.Repeat(" ", 12-len(name)), nums)
					}

					return nil
				}

				if err := printStats(partition.AllSectors, "All"); err != nil {
					return err
				}
				if err := printStats(partition.LiveSectors, "Live"); err != nil {
					return err
				}
				if err := printStats(partition.ActiveSectors, "Active"); err != nil {
					return err
				}
				if err := printStats(partition.FaultySectors, "Faulty"); err != nil {
					return err
				}
				if err := printStats(partition.RecoveringSectors, "Recovering"); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func ProvingFaultsCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:  "faults",
		Usage: "View the currently known proving faulty sectors information",
		Action: func(cctx *cli.Context) error {
			api, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			stor := store.ActorStore(ctx, blockstore.NewAPIBlockstore(api))

			maddr, err := getActorAddress(cctx)
			if err != nil {
				return err
			}

			mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			mas, err := miner.Load(stor, mact)
			if err != nil {
				return err
			}

			fmt.Printf("Miner: %s\n", color.BlueString("%s", maddr))

			tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "deadline\tpartition\tsectors")
			err = mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
				return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
					faults, err := part.FaultySectors()
					if err != nil {
						return err
					}
					return faults.ForEach(func(num uint64) error {
						_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\n", dlIdx, partIdx, num)
						return nil
					})
				})
			})
			if err != nil {
				return err
			}
			return tw.Flush()
		},
	}
}
