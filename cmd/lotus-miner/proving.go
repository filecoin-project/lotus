package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var provingCmd = &cli.Command{
	Name:  "proving",
	Usage: "View proving information",
	Subcommands: []*cli.Command{
		provingInfoCmd,
		provingDeadlinesCmd,
		provingDeadlineInfoCmd,
		provingFaultsCmd,
		provingCheckProvableCmd,
		workersCmd(false),
		provingComputeCmd,
		provingRecoverFaultsCmd,
	},
}

var provingFaultsCmd = &cli.Command{
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

		maddr, err := getActorAddress(ctx, cctx)
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

var provingInfoCmd = &cli.Command{
	Name:  "info",
	Usage: "View current state information",
	Action: func(cctx *cli.Context) error {
		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := getActorAddress(ctx, cctx)
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

var provingDeadlinesCmd = &cli.Command{
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

		maddr, err := getActorAddress(ctx, cctx)
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

		fmt.Printf("Miner: %s\n", color.BlueString("%s", maddr))

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "deadline\tpartitions\tsectors (faults)\tproven partitions")

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
			_, _ = fmt.Fprintf(tw, "%d\t%d\t%d (%d)\t%d%s\n", dlIdx, partitionCount, sectors, faults, provenPartitions, cur)
		}

		return tw.Flush()
	},
}

var provingDeadlineInfoCmd = &cli.Command{
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

		maddr, err := getActorAddress(ctx, cctx)
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

		provenPartitions, err := deadlines[dlIdx].PostSubmissions.Count()
		if err != nil {
			return err
		}

		fmt.Printf("Deadline Index:           %d\n", dlIdx)
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

var provingCheckProvableCmd = &cli.Command{
	Name:      "check",
	Usage:     "Check sectors provable",
	ArgsUsage: "<deadlineIdx>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "only-bad",
			Usage: "print only bad sectors",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "slow",
			Usage: "run slower checks",
		},
		&cli.StringFlag{
			Name:  "storage-id",
			Usage: "filter sectors by storage path (path id)",
		},
		&cli.BoolFlag{
			Name:  "faulty",
			Usage: "only check faulty sectors",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		dlIdx, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse deadline index: %w", err)
		}

		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()

		minerApi, scloser, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer scloser()

		ctx := lcli.ReqContext(cctx)

		addr, err := minerApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		mid, err := address.IDFromAddress(addr)
		if err != nil {
			return err
		}

		info, err := api.StateMinerInfo(ctx, addr, types.EmptyTSK)
		if err != nil {
			return err
		}

		partitions, err := api.StateMinerPartitions(ctx, addr, dlIdx, types.EmptyTSK)
		if err != nil {
			return err
		}

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "deadline\tpartition\tsector\tstatus")

		var filter map[abi.SectorID]struct{}

		if cctx.IsSet("storage-id") {
			sl, err := minerApi.StorageList(ctx)
			if err != nil {
				return err
			}
			decls := sl[storiface.ID(cctx.String("storage-id"))]

			filter = map[abi.SectorID]struct{}{}
			for _, decl := range decls {
				filter[decl.SectorID] = struct{}{}
			}
		}

		if cctx.Bool("faulty") {
			parts, err := getAllPartitions(ctx, addr, api)
			if err != nil {
				return xerrors.Errorf("getting partitions: %w", err)
			}

			if filter != nil {
				for k := range filter {
					set, err := parts.FaultySectors.IsSet(uint64(k.Number))
					if err != nil {
						return err
					}
					if !set {
						delete(filter, k)
					}
				}
			} else {
				filter = map[abi.SectorID]struct{}{}

				err = parts.FaultySectors.ForEach(func(s uint64) error {
					filter[abi.SectorID{
						Miner:  abi.ActorID(mid),
						Number: abi.SectorNumber(s),
					}] = struct{}{}
					return nil
				})
				if err != nil {
					return err
				}
			}
		}

		for parIdx, par := range partitions {
			sectors := make(map[abi.SectorNumber]struct{})

			sectorInfos, err := api.StateMinerSectors(ctx, addr, &par.LiveSectors, types.EmptyTSK)
			if err != nil {
				return err
			}

			var tocheck []storiface.SectorRef
			for _, info := range sectorInfos {
				si := abi.SectorID{
					Miner:  abi.ActorID(mid),
					Number: info.SectorNumber,
				}

				if filter != nil {
					if _, found := filter[si]; !found {
						continue
					}
				}

				sectors[info.SectorNumber] = struct{}{}
				tocheck = append(tocheck, storiface.SectorRef{
					ProofType: info.SealProof,
					ID:        si,
				})
			}

			bad, err := minerApi.CheckProvable(ctx, info.WindowPoStProofType, tocheck)
			if err != nil {
				return err
			}

			for s := range sectors {
				if err, exist := bad[s]; exist {
					_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%s\n", dlIdx, parIdx, s, color.RedString("bad")+fmt.Sprintf(" (%s)", err))
				} else if !cctx.Bool("only-bad") {
					_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%s\n", dlIdx, parIdx, s, color.GreenString("good"))
				}
			}
		}

		return tw.Flush()
	},
}

var provingComputeCmd = &cli.Command{
	Name:  "compute",
	Usage: "Compute simulated proving tasks",
	Subcommands: []*cli.Command{
		provingComputeWindowPoStCmd,
	},
}

var provingComputeWindowPoStCmd = &cli.Command{
	Name:    "windowed-post",
	Aliases: []string{"window-post"},
	Usage:   "Compute WindowPoSt for a specific deadline",
	Description: `Note: This command is intended to be used to verify PoSt compute performance.
It will not send any messages to the chain.`,
	ArgsUsage: "[deadline index]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		dlIdx, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse deadline index: %w", err)
		}

		minerApi, scloser, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer scloser()

		ctx := lcli.ReqContext(cctx)

		start := time.Now()
		res, err := minerApi.ComputeWindowPoSt(ctx, dlIdx, types.EmptyTSK)
		fmt.Printf("Took %s\n", time.Now().Sub(start))
		if err != nil {
			return err
		}

		//convert sector information into easily readable information
		type PoStPartition struct {
			Index   uint64
			Skipped []uint64
		}
		type SubmitWindowedPoStParams struct {
			Deadline         uint64
			Partitions       []PoStPartition
			Proofs           []proof.PoStProof
			ChainCommitEpoch abi.ChainEpoch
			ChainCommitRand  abi.Randomness
		}
		var postParams []SubmitWindowedPoStParams
		for _, i := range res {
			var postParam SubmitWindowedPoStParams
			postParam.Deadline = i.Deadline
			for id, part := range i.Partitions {
				postParam.Partitions[id].Index = part.Index
				count, err := part.Skipped.Count()
				if err != nil {
					return err
				}
				sectors, err := part.Skipped.All(count)
				if err != nil {
					return err
				}
				postParam.Partitions[id].Skipped = sectors
			}
			postParam.Proofs = i.Proofs
			postParam.ChainCommitEpoch = i.ChainCommitEpoch
			postParam.ChainCommitRand = i.ChainCommitRand
			postParams = append(postParams, postParam)
		}

		jr, err := json.MarshalIndent(postParams, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(jr))

		return nil
	},
}

var provingRecoverFaultsCmd = &cli.Command{
	Name:      "recover-faults",
	Usage:     "Manually recovers faulty sectors on chain",
	ArgsUsage: "<faulty sectors>",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "confidence",
			Usage: "number of block confirmations to wait for",
			Value: int(build.MessageConfidence),
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() < 1 {
			return lcli.ShowHelp(cctx, xerrors.Errorf("must pass at least 1 sector number"))
		}

		arglist := cctx.Args().Slice()
		var sectors []abi.SectorNumber
		for _, v := range arglist {
			s, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return xerrors.Errorf("failed to convert sectors, please check the arguments: %w", err)
			}
			sectors = append(sectors, abi.SectorNumber(s))
		}

		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		msgs, err := minerApi.RecoverFault(ctx, sectors)
		if err != nil {
			return err
		}

		// wait for msgs to get mined into a block
		var wg sync.WaitGroup
		wg.Add(len(msgs))
		results := make(chan error, len(msgs))
		for _, msg := range msgs {
			go func(m cid.Cid) {
				defer wg.Done()
				wait, err := api.StateWaitMsg(ctx, m, uint64(cctx.Int("confidence")))
				if err != nil {
					results <- xerrors.Errorf("Timeout waiting for message to land on chain %s", wait.Message)
					return
				}

				if wait.Receipt.ExitCode.IsError() {
					results <- xerrors.Errorf("Failed to execute message %s: %w", wait.Message, wait.Receipt.ExitCode.Error())
					return
				}
				results <- nil
				return
			}(msg)
		}

		wg.Wait()
		close(results)

		for v := range results {
			if v != nil {
				fmt.Println("Failed to execute the message %w", v)
			}
		}
		return nil
	},
}
