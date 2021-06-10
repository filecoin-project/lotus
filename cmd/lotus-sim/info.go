package main

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
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

var infoSimCommand = &cli.Command{
	Name:        "info",
	Description: "Output information about the simulation.",
	Action: func(cctx *cli.Context) error {
		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer node.Close()

		sim, err := node.LoadSim(cctx.Context, cctx.String("simulation"))
		if err != nil {
			return err
		}

		powerNow, err := getTotalPower(cctx.Context, sim.StateManager, sim.GetHead())
		if err != nil {
			return err
		}
		powerStart, err := getTotalPower(cctx.Context, sim.StateManager, sim.GetStart())
		if err != nil {
			return err
		}
		powerGrowth := big.Sub(powerNow.RawBytePower, powerStart.RawBytePower)

		tw := tabwriter.NewWriter(cctx.App.Writer, 8, 8, 0, ' ', 0)

		head := sim.GetHead()
		start := sim.GetStart()
		headEpoch := head.Height()
		firstEpoch := start.Height() + 1

		headTime := time.Unix(int64(head.MinTimestamp()), 0)
		startTime := time.Unix(int64(start.MinTimestamp()), 0)
		duration := headTime.Sub(startTime)

		// growth rate in size/day
		growthRate := big.Div(
			big.Mul(powerGrowth, big.NewInt(int64(24*time.Hour))),
			big.NewInt(int64(duration)),
		)

		fmt.Fprintf(tw, "Name:\t%s\n", sim.Name())
		fmt.Fprintf(tw, "Head:\t%s\n", head)
		fmt.Fprintf(tw, "Last Epoch:\t%d\n", headEpoch)
		fmt.Fprintf(tw, "First Epoch:\t%d\n", firstEpoch)
		fmt.Fprintf(tw, "Length:\t%d\n", headEpoch-firstEpoch)
		fmt.Fprintf(tw, "Date:\t%s\n", headTime)
		fmt.Fprintf(tw, "Duration:\t%s\n", duration)
		fmt.Fprintf(tw, "Power:\t%s\n", types.SizeStr(powerNow.RawBytePower))
		fmt.Fprintf(tw, "Power Growth:\t%s\n", types.SizeStr(powerGrowth))
		fmt.Fprintf(tw, "Power Growth Rate:\t%s/day\n", types.SizeStr(growthRate))
		fmt.Fprintf(tw, "Network Version:\t%d\n", sim.GetNetworkVersion())
		return tw.Flush()
	},
}
