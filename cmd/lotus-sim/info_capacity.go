package main

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

var infoCapacityGrowthSimCommand = &cli.Command{
	Name:        "capacity-growth",
	Description: "List daily capacity growth over the course of the simulation starting at the end.",
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

		firstEpoch := sim.GetStart().Height()
		ts := sim.GetHead()
		lastPower, err := getTotalPower(cctx.Context, sim.StateManager, ts)
		if err != nil {
			return err
		}
		lastHeight := ts.Height()

		for ts.Height() > firstEpoch && cctx.Err() == nil {
			ts, err = sim.Node.Chainstore.LoadTipSet(cctx.Context, ts.Parents())
			if err != nil {
				return err
			}
			newEpoch := ts.Height()
			if newEpoch != firstEpoch && newEpoch+builtin.EpochsInDay > lastHeight {
				continue
			}

			newPower, err := getTotalPower(cctx.Context, sim.StateManager, ts)
			if err != nil {
				return err
			}

			growthRate := big.Div(
				big.Mul(big.Sub(lastPower.RawBytePower, newPower.RawBytePower),
					big.NewInt(builtin.EpochsInDay)),
				big.NewInt(int64(lastHeight-newEpoch)),
			)
			lastPower = newPower
			lastHeight = newEpoch
			_, _ = fmt.Fprintf(cctx.App.Writer, "%s/day\n", types.SizeStr(growthRate))
		}
		return cctx.Err()
	},
}
