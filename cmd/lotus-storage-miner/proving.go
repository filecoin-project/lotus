package main

import (
	"bytes"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var provingCmd = &cli.Command{
	Name:  "proving",
	Usage: "View proving information",
	Subcommands: []*cli.Command{
		provingInfoCmd,
		provingDeadlinesCmd,
		provingFaultsCmd,
	},
}

var provingFaultsCmd = &cli.Command{
	Name:  "faults",
	Usage: "View the currently known proving faulty sectors information",
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
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

		maddr, err := getActorAddress(ctx, nodeApi, cctx.String("actor"))
		if err != nil {
			return err
		}

		var mas miner.State
		{
			mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}
			rmas, err := api.ChainReadObj(ctx, mact.Head)
			if err != nil {
				return err
			}
			if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
				return err
			}
		}

		fmt.Printf("Miner: %s\n", color.BlueString("%s", maddr))

		head, err := api.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}
		deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
		if err != nil {
			return xerrors.Errorf("getting miner deadlines: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "deadline\tpartition\tsectors")
		for dlIdx := range deadlines {
			partitions, err := api.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("loading partitions for deadline %d: %w", dlIdx, err)
			}

			for partIdx, partition := range partitions {
				faulty, err := partition.Faults.All(10000000)
				if err != nil {
					return err
				}

				for _, num := range faulty {
					_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\n", dlIdx, partIdx, num)
				}
			}
		}
		return tw.Flush()
	},
}

var provingInfoCmd = &cli.Command{
	Name:  "info",
	Usage: "View current state information",
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
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

		maddr, err := getActorAddress(ctx, nodeApi, cctx.String("actor"))
		if err != nil {
			return err
		}

		head, err := api.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}

		cd, err := api.StateMinerProvingDeadline(ctx, maddr, head.Key())
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
		if err != nil {
			return xerrors.Errorf("getting miner deadlines: %w", err)
		}

		fmt.Printf("Miner: %s\n", color.BlueString("%s", maddr))

		var mas miner.State
		{
			mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}
			rmas, err := api.ChainReadObj(ctx, mact.Head)
			if err != nil {
				return err
			}
			if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
				return err
			}
		}

		parts := map[uint64][]*miner.Partition{}
		for dlIdx := range deadlines {
			part, err := api.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting miner partition: %w", err)
			}

			parts[uint64(dlIdx)] = part
		}

		proving := uint64(0)
		faults := uint64(0)
		recovering := uint64(0)

		for _, partitions := range parts {
			for _, partition := range partitions {
				sc, err := partition.Sectors.Count()
				if err != nil {
					return xerrors.Errorf("count partition sectors: %w", err)
				}
				proving += sc

				fc, err := partition.Faults.Count()
				if err != nil {
					return xerrors.Errorf("count partition faults: %w", err)
				}
				faults += fc

				rc, err := partition.Recoveries.Count()
				if err != nil {
					return xerrors.Errorf("count partition recoveries: %w", err)
				}
				recovering += rc
			}
		}

		var faultPerc float64
		if proving > 0 {
			faultPerc = float64(faults*10000/proving) / 100
		}

		fmt.Printf("Current Epoch:           %d\n", cd.CurrentEpoch)

		fmt.Printf("Proving Period Boundary: %d\n", cd.PeriodStart%miner.WPoStProvingPeriod)
		fmt.Printf("Proving Period Start:    %s\n", epochTime(cd.CurrentEpoch, cd.PeriodStart))
		fmt.Printf("Next Period Start:       %s\n\n", epochTime(cd.CurrentEpoch, cd.PeriodStart+miner.WPoStProvingPeriod))

		fmt.Printf("Faults:      %d (%.2f%%)\n", faults, faultPerc)
		fmt.Printf("Recovering:  %d\n", recovering)

		fmt.Printf("Deadline Index:       %d\n", cd.Index)

		if cd.Index < miner.WPoStPeriodDeadlines {
			curDeadlineSectors := uint64(0)
			for _, partition := range parts[cd.Index] {
				sc, err := partition.Sectors.Count()
				if err != nil {
					return xerrors.Errorf("counting current deadline sectors: %w", err)
				}
				curDeadlineSectors += sc
			}

			fmt.Printf("Deadline Sectors:     %d\n", curDeadlineSectors)
		}

		fmt.Printf("Deadline Open:        %s\n", epochTime(cd.CurrentEpoch, cd.Open))
		fmt.Printf("Deadline Close:       %s\n", epochTime(cd.CurrentEpoch, cd.Close))
		fmt.Printf("Deadline Challenge:   %s\n", epochTime(cd.CurrentEpoch, cd.Challenge))
		fmt.Printf("Deadline FaultCutoff: %s\n", epochTime(cd.CurrentEpoch, cd.FaultCutoff))
		return nil
	},
}

func epochTime(curr, e abi.ChainEpoch) string {
	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e)))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr)))
	}

	panic("math broke")
}

var provingDeadlinesCmd = &cli.Command{
	Name:  "deadlines",
	Usage: "View the current proving period deadlines information",
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
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

		maddr, err := getActorAddress(ctx, nodeApi, cctx.String("actor"))
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

		var mas miner.State
		{
			mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}
			rmas, err := api.ChainReadObj(ctx, mact.Head)
			if err != nil {
				return err
			}
			if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
				return err
			}
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

			for _, partition := range partitions {
				sc, err := partition.Sectors.Count()
				if err != nil {
					return err
				}

				sectors += sc

				fc, err := partition.Faults.Count()
				if err != nil {
					return err
				}

				faults += fc
			}

			var cur string
			if di.Index == uint64(dlIdx) {
				cur += "\t(current)"
			}
			_, _ = fmt.Fprintf(tw, "%d\t%d\t%d (%d)\t%d%s\n", dlIdx, len(partitions), sectors, faults, provenPartitions, cur)
		}

		return tw.Flush()
	},
}
