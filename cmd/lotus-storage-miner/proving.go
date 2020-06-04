package main

import (
	"bytes"
	"fmt"
	"time"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"
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

		for i, field := range deadlines.Due {
			c, err := field.Count()
			if err != nil {
				return err
			}

			var info string
			proven, err := mas.PostSubmissions.IsSet(uint64(i))
			if err != nil {
				return err
			}

			if proven {
				info += ", proven"
			}

			if di.Index == uint64(i) {
				info += " (current)"
			}
			fmt.Printf("%d: %d sectors%s\n", i, c, info)
		}

		return nil
	},
}
