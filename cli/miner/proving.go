package miner

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/spcli"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var provingCmd = &cli.Command{
	Name:  "proving",
	Usage: "View proving information",
	Subcommands: []*cli.Command{
		spcli.ProvingInfoCmd(LMActorOrEnvGetter),
		spcli.ProvingDeadlinesCmd(LMActorOrEnvGetter),
		spcli.ProvingDeadlineInfoCmd(LMActorOrEnvGetter),
		spcli.ProvingFaultsCmd(LMActorOrEnvGetter),
		provingCheckProvableCmd,
		workersCmd(false),
		provingComputeCmd,
		provingRecoverFaultsCmd,
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

			tmp := par.LiveSectors
			sectorInfos, err := api.StateMinerSectors(ctx, addr, &tmp, types.EmptyTSK)
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
		fmt.Printf("Took %s\n", time.Since(start))
		if err != nil {
			return err
		}

		// convert sector information into easily readable information
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

			// Initialize the postParam.Partitions slice with the same length as i.Partitions
			postParam.Partitions = make([]PoStPartition, len(i.Partitions))

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
			Value: int(buildconstants.MessageConfidence),
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
