package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"

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
	powerNow, err := getTotalPower(ctx, sim.StateManager, sim.GetHead())
	if err != nil {
		return err
	}
	powerStart, err := getTotalPower(ctx, sim.StateManager, sim.GetStart())
	if err != nil {
		return err
	}
	powerGrowth := big.Sub(powerNow.RawBytePower, powerStart.RawBytePower)

	tw := tabwriter.NewWriter(out, 8, 8, 1, ' ', 0)

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
}

var infoSimCommand = &cli.Command{
	Name:        "info",
	Description: "Output information about the simulation.",
	Subcommands: []*cli.Command{
		infoCommitGasSimCommand,
	},
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
		return printInfo(cctx.Context, sim, cctx.App.Writer)
	},
}

var infoCommitGasSimCommand = &cli.Command{
	Name:        "commit-gas",
	Description: "Output information about the gas for committs",
	Action: func(cctx *cli.Context) error {
		log := func(f string, i ...interface{}) {
			fmt.Fprintf(os.Stderr, f, i...)
		}
		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer node.Close()

		sim, err := node.LoadSim(cctx.Context, cctx.String("simulation"))
		if err != nil {
			return err
		}

		var gasAgg, proofsAgg uint64
		var gasAggMax, proofsAggMax uint64

		sim.Walk(cctx.Context, func(sm *stmgr.StateManager, ts *types.TipSet, stCid cid.Cid,
			messages []*simulation.AppliedMessage) error {
			for _, m := range messages {
				if m.ExitCode != exitcode.Ok {
					continue
				}
				if m.Method == builtin.MethodsMiner.ProveCommitAggregate {
					param := miner.ProveCommitAggregateParams{}
					err := param.UnmarshalCBOR(bytes.NewReader(m.Params))
					if err != nil {
						log("failed to decode params: %+v", err)
						return nil
					}
					c, err := param.SectorNumbers.Count()
					if err != nil {
						log("failed to count sectors")
						return nil
					}
					gasAgg += uint64(m.GasUsed)
					proofsAgg += c
					if c == 819 {
						gasAggMax += uint64(m.GasUsed)
						proofsAggMax += c
					}
				}

				if m.Method == builtin.MethodsMiner.ProveCommitSector {
				}
			}

			return nil
		})
		idealGassUsed := float64(gasAggMax) / float64(proofsAggMax) * float64(proofsAgg)

		fmt.Printf("Gas usage efficiency in comparison to all 819: %f%%\n", 100*idealGassUsed/float64(gasAgg))

		return nil
	},
}
