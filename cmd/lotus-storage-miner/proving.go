package main

import (
	"fmt"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"
	"time"
)

var provingCmd = &cli.Command{
	Name: "proving",
	Subcommands: []*cli.Command{
		provingInfoCmd,
		provingDeadlinesCmd,
	},
}

var provingInfoCmd = &cli.Command{
	Name: "info",
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

		mi, err := api.StateMinerInfo(ctx, maddr, head.Key())
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		cd, _ := miner.ComputeProvingPeriodDeadline(mi.ProvingPeriodBoundary, head.Height())

		deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
		if err != nil {
			return xerrors.Errorf("getting miner deadlines: %w", err)
		}

		curDeadlineSectors, err := deadlines.Due[cd.Index].Count()
		if err != nil {
			return xerrors.Errorf("counting deadline sectors: %w", err)
		}

		fmt.Printf("Current Epoch:           %d\n", cd.CurrentEpoch)
		fmt.Printf("Chain Period:            %d\n", cd.CurrentEpoch/miner.WPoStProvingPeriod)
		fmt.Printf("Chain Period Start:      %s\n", epochTime(cd.CurrentEpoch, (cd.CurrentEpoch/miner.WPoStProvingPeriod)*miner.WPoStProvingPeriod))
		fmt.Printf("Chain Period End:        %s\n\n", epochTime(cd.CurrentEpoch, (cd.CurrentEpoch/miner.WPoStProvingPeriod+1)*miner.WPoStProvingPeriod))

		fmt.Printf("Proving Period Boundary: %d\n", mi.ProvingPeriodBoundary)
		fmt.Printf("Proving Period Start:    %s\n", epochTime(cd.CurrentEpoch, cd.PeriodStart))
		fmt.Printf("Next Period Start:       %s\n\n", epochTime(cd.CurrentEpoch, cd.PeriodStart+miner.WPoStProvingPeriod))

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
		return fmt.Sprintf("%d (%s ago)", e, time.Second*time.Duration(build.BlockDelay*(curr-e)))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, time.Second*time.Duration(build.BlockDelay*(e-curr)))
	}

	panic("math broke")
}

var provingDeadlinesCmd = &cli.Command{
	Name: "deadlines",
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

		for i, field := range deadlines.Due {
			c, err := field.Count()
			if err != nil {
				return err
			}

			fmt.Printf("%d: %d sectors\n", i, c)
		}

		return nil
	},
}
