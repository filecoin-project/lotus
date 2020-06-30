package main

import (
	"bytes"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	"github.com/filecoin-project/specs-actors/actors/abi"
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

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return xerrors.Errorf("getting actor address: %w", err)
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
		faults, err := mas.Faults.All(100000000000)
		if err != nil {
			return err
		}
		if len(faults) == 0 {
			fmt.Println("no faulty sectors")
		}
		head, err := api.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}
		deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
		if err != nil {
			return xerrors.Errorf("getting miner deadlines: %w", err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "deadline\tsectors")
		for deadline, sectors := range deadlines.Due {
			intersectSectors, _ := bitfield.IntersectBitField(sectors, mas.Faults)
			if intersectSectors != nil {
				allSectors, _ := intersectSectors.All(100000000000)
				for _, num := range allSectors {
					_, _ = fmt.Fprintf(tw, "%d\t%d\n", deadline, num)
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

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return xerrors.Errorf("getting actor address: %w", err)
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

		curDeadlineSectors, err := deadlines.Due[cd.Index].Count()
		if err != nil {
			return xerrors.Errorf("counting deadline sectors: %w", err)
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

		newSectors, err := mas.NewSectors.Count()
		if err != nil {
			return err
		}

		faults, err := mas.Faults.Count()
		if err != nil {
			return err
		}

		recoveries, err := mas.Recoveries.Count()
		if err != nil {
			return err
		}

		var provenSectors uint64
		for _, d := range deadlines.Due {
			c, err := d.Count()
			if err != nil {
				return err
			}
			provenSectors += c
		}

		var faultPerc float64
		if provenSectors > 0 {
			faultPerc = float64(faults*10000/provenSectors) / 100
		}

		fmt.Printf("Current Epoch:           %d\n", cd.CurrentEpoch)
		fmt.Printf("Chain Period:            %d\n", cd.CurrentEpoch/miner.WPoStProvingPeriod)
		fmt.Printf("Chain Period Start:      %s\n", epochTime(cd.CurrentEpoch, (cd.CurrentEpoch/miner.WPoStProvingPeriod)*miner.WPoStProvingPeriod))
		fmt.Printf("Chain Period End:        %s\n\n", epochTime(cd.CurrentEpoch, (cd.CurrentEpoch/miner.WPoStProvingPeriod+1)*miner.WPoStProvingPeriod))

		fmt.Printf("Proving Period Boundary: %d\n", cd.PeriodStart%miner.WPoStProvingPeriod)
		fmt.Printf("Proving Period Start:    %s\n", epochTime(cd.CurrentEpoch, cd.PeriodStart))
		fmt.Printf("Next Period Start:       %s\n\n", epochTime(cd.CurrentEpoch, cd.PeriodStart+miner.WPoStProvingPeriod))

		fmt.Printf("Faults:      %d (%.2f%%)\n", faults, faultPerc)
		fmt.Printf("Recovering:  %d\n", recoveries)
		fmt.Printf("New Sectors: %d\n\n", newSectors)

		fmt.Printf("Deadline Index:       %d\n", cd.Index)
		fmt.Printf("Deadline Sectors:     %d\n", curDeadlineSectors)
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

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return xerrors.Errorf("getting actor address: %w", err)
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

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "deadline\tsectors\tpartitions\tproven")

		for i, field := range deadlines.Due {
			c, err := field.Count()
			if err != nil {
				return err
			}

			firstPartition, sectorCount, err := miner.PartitionsForDeadline(deadlines, mas.Info.WindowPoStPartitionSectors, uint64(i))
			if err != nil {
				return err
			}

			partitionCount := (sectorCount + mas.Info.WindowPoStPartitionSectors - 1) / mas.Info.WindowPoStPartitionSectors

			var provenPartitions uint64
			{
				var maskRuns []rlepluslazy.Run
				if firstPartition > 0 {
					maskRuns = append(maskRuns, rlepluslazy.Run{
						Val: false,
						Len: firstPartition,
					})
				}
				maskRuns = append(maskRuns, rlepluslazy.Run{
					Val: true,
					Len: partitionCount,
				})

				ppbm, err := bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{Runs: maskRuns})
				if err != nil {
					return err
				}

				pp, err := bitfield.IntersectBitField(ppbm, mas.PostSubmissions)
				if err != nil {
					return err
				}

				provenPartitions, err = pp.Count()
				if err != nil {
					return err
				}
			}

			var cur string
			if di.Index == uint64(i) {
				cur += "\t(current)"
			}
			_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%d%s\n", i, c, partitionCount, provenPartitions, cur)
		}

		return tw.Flush()
	},
}
