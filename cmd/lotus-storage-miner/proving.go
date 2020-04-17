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

		pps, _ := (&miner.State{Info: mi}).ProvingPeriodStart(head.Height())
		npp := pps + miner.WPoStProvingPeriod

		cd, chg := miner.ComputeCurrentDeadline(pps, head.Height())

		deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
		if err != nil {
			return xerrors.Errorf("getting miner deadlines: %w", err)
		}

		curDeadlineSectors, err := deadlines.Due[cd].Count()
		if err != nil {
			return xerrors.Errorf("counting deadline sectors: %w", err)
		}

		fmt.Printf("Proving Period Boundary: %d\n", mi.ProvingPeriodBoundary)
		fmt.Printf("Current Epoch:           %d\n", head.Height())
		fmt.Printf("Proving Period Start:    %d (%s ago)\n", pps, time.Second*time.Duration(build.BlockDelay*(head.Height()-pps)))
		fmt.Printf("Next Proving Period:     %d (in %s)\n\n", npp, time.Second*time.Duration(build.BlockDelay*(npp-head.Height())))

		fmt.Printf("Deadline:         %d\n", cd)
		fmt.Printf("Deadline Sectors: %d\n", curDeadlineSectors)
		fmt.Printf("Deadline Start:   %d\n", pps+(abi.ChainEpoch(cd)*miner.WPoStChallengeWindow))
		fmt.Printf("Challenge Epoch:  %d\n", chg)
		fmt.Printf("Time left:        %s\n", time.Second*time.Duration(build.BlockDelay*((pps+((1+abi.ChainEpoch(cd))*miner.WPoStChallengeWindow))-head.Height())))
		return nil
	},
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
