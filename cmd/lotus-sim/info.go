package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation"
)

func getTotalPower(ctx context.Context, sm *stmgr.StateManager, ts *types.TipSet) (power.Claim, error) {
	actor, err := sm.LoadActor(ctx, power.Address, ts)
	if err != nil {
		return power.Claim{}, err
	}
	state, err := power.Load(sm.ChainStore().ActorStore(ctx), actor)
	if err != nil {
		return power.Claim{}, err
	}
	return state.TotalPower()
}

func printInfo(ctx context.Context, sim *simulation.Simulation, out io.Writer) error {
	head := sim.GetHead()
	start := sim.GetStart()

	powerNow, err := getTotalPower(ctx, sim.StateManager, head)
	if err != nil {
		return err
	}
	powerLookbackEpoch := head.Height() - builtin.EpochsInDay*2
	if powerLookbackEpoch < start.Height() {
		powerLookbackEpoch = start.Height()
	}
	lookbackTs, err := sim.Node.Chainstore.GetTipsetByHeight(ctx, powerLookbackEpoch, head, false)
	if err != nil {
		return err
	}
	powerLookback, err := getTotalPower(ctx, sim.StateManager, lookbackTs)
	if err != nil {
		return err
	}
	// growth rate in size/day
	growthRate := big.Div(
		big.Mul(big.Sub(powerNow.RawBytePower, powerLookback.RawBytePower),
			big.NewInt(builtin.EpochsInDay)),
		big.NewInt(int64(head.Height()-lookbackTs.Height())),
	)

	tw := tabwriter.NewWriter(out, 8, 8, 1, ' ', 0)

	headEpoch := head.Height()
	firstEpoch := start.Height() + 1

	headTime := time.Unix(int64(head.MinTimestamp()), 0)
	startTime := time.Unix(int64(start.MinTimestamp()), 0)
	duration := headTime.Sub(startTime)

	fmt.Fprintf(tw, "Num:\t%s\n", sim.Name())                                      //nolint:errcheck
	fmt.Fprintf(tw, "Head:\t%s\n", head)                                           //nolint:errcheck
	fmt.Fprintf(tw, "Start Epoch:\t%d\n", firstEpoch)                              //nolint:errcheck
	fmt.Fprintf(tw, "End Epoch:\t%d\n", headEpoch)                                 //nolint:errcheck
	fmt.Fprintf(tw, "Length:\t%d\n", headEpoch-firstEpoch)                         //nolint:errcheck
	fmt.Fprintf(tw, "Start Date:\t%s\n", startTime)                                //nolint:errcheck
	fmt.Fprintf(tw, "End Date:\t%s\n", headTime)                                   //nolint:errcheck
	fmt.Fprintf(tw, "Duration:\t%.2f day(s)\n", duration.Hours()/24)               //nolint:errcheck
	fmt.Fprintf(tw, "Capacity:\t%s\n", types.SizeStr(powerNow.RawBytePower))       //nolint:errcheck
	fmt.Fprintf(tw, "Daily Capacity Growth:\t%s/day\n", types.SizeStr(growthRate)) //nolint:errcheck
	fmt.Fprintf(tw, "Network Version:\t%d\n", sim.GetNetworkVersion())             //nolint:errcheck
	return tw.Flush()
}

var infoSimCommand = &cli.Command{
	Name:        "info",
	Description: "Output information about the simulation.",
	Subcommands: []*cli.Command{
		infoCommitGasSimCommand,
		infoMessageSizeSimCommand,
		infoWindowPostBandwidthSimCommand,
		infoCapacityGrowthSimCommand,
		infoStateGrowthSimCommand,
	},
	Action: func(cctx *cli.Context) (err error) {
		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := node.Close(); err == nil {
				err = cerr
			}
		}()

		sim, err := node.LoadSim(cctx.Context, cctx.String("simulation"))
		if err != nil {
			return err
		}
		return printInfo(cctx.Context, sim, cctx.App.Writer)
	},
}
