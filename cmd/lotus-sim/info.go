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
	"github.com/streadway/quantile"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"

	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation"
	"github.com/filecoin-project/lotus/lib/stati"
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
	fmt.Fprintf(tw, "Start Epoch:\t%d\n", firstEpoch)
	fmt.Fprintf(tw, "End Epoch:\t%d\n", headEpoch)
	fmt.Fprintf(tw, "Length:\t%d\n", headEpoch-firstEpoch)
	fmt.Fprintf(tw, "Start Date:\t%s\n", startTime)
	fmt.Fprintf(tw, "End Date:\t%s\n", headTime)
	fmt.Fprintf(tw, "Duration:\t%.2f day(s)\n", duration.Hours()/24)
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
		infoWindowPostBandwidthSimCommand,
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

var infoWindowPostBandwidthSimCommand = &cli.Command{
	Name:        "post-bandwidth",
	Description: "List average chain bandwidth used by window posts for each day of the simulation.",
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

		var postGas, totalGas int64
		printStats := func() {
			fmt.Fprintf(cctx.App.Writer, "%.4f%%\n", float64(100*postGas)/float64(totalGas))
		}
		idx := 0
		err = sim.Walk(cctx.Context, 0, func(
			sm *stmgr.StateManager, ts *types.TipSet, stCid cid.Cid,
			messages []*simulation.AppliedMessage,
		) error {
			for _, m := range messages {
				totalGas += m.GasUsed
				if m.ExitCode != exitcode.Ok {
					continue
				}
				if m.Method == builtin.MethodsMiner.SubmitWindowedPoSt {
					postGas += m.GasUsed
				}
			}
			idx++
			idx %= builtin.EpochsInDay
			if idx == 0 {
				printStats()
				postGas = 0
				totalGas = 0
			}
			return nil
		})
		if idx > 0 {
			printStats()
		}
		return err
	},
}

var infoCommitGasSimCommand = &cli.Command{
	Name:        "commit-gas",
	Description: "Output information about the gas for commits",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "lookback",
			Value: 0,
		},
	},
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
		var gasSingle, proofsSingle uint64

		qpoints := []struct{ q, tol float64 }{
			{0.01, 0.0005},
			{0.05, 0.001},
			{0.20, 0.01},
			{0.25, 0.01},
			{0.30, 0.01},
			{0.40, 0.01},
			{0.45, 0.01},
			{0.50, 0.01},
			{0.60, 0.01},
			{0.80, 0.01},
			{0.95, 0.001},
			{0.99, 0.0005},
		}
		estims := make([]quantile.Estimate, len(qpoints))
		for i, p := range qpoints {
			estims[i] = quantile.Known(p.q, p.tol)
		}
		qua := quantile.New(estims...)
		hist, err := stati.NewHistogram([]float64{
			1, 3, 5, 7, 15, 30, 50, 100, 200, 400, 600, 700, 819})
		if err != nil {
			return err
		}

		err = sim.Walk(cctx.Context, cctx.Int64("lookback"), func(
			sm *stmgr.StateManager, ts *types.TipSet, stCid cid.Cid,
			messages []*simulation.AppliedMessage,
		) error {
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
					for i := uint64(0); i < c; i++ {
						qua.Add(float64(c))
					}
					hist.Observe(float64(c))
				}

				if m.Method == builtin.MethodsMiner.ProveCommitSector {
					gasSingle += uint64(m.GasUsed)
					proofsSingle++
					qua.Add(1)
					hist.Observe(1)
				}
			}

			return nil
		})
		if err != nil {
			return err
		}
		idealGassUsed := float64(gasAggMax) / float64(proofsAggMax) * float64(proofsAgg+proofsSingle)

		fmt.Printf("Gas usage efficiency in comparison to all 819: %f%%\n", 100*idealGassUsed/float64(gasAgg+gasSingle))

		fmt.Printf("Proofs in singles: %d\n", proofsSingle)
		fmt.Printf("Proofs in Aggs: %d\n", proofsAgg)
		fmt.Printf("Proofs in Aggs(819): %d\n", proofsAggMax)

		fmt.Println()
		fmt.Println("Quantiles of proofs in given aggregate size:")
		for _, p := range qpoints {
			fmt.Printf("%.0f%%\t%.0f\n", p.q*100, qua.Get(p.q))
		}
		fmt.Println()
		fmt.Println("Histogram of messages:")
		fmt.Printf("Total\t%d\n", hist.Total())
		for i, b := range hist.Buckets[1:] {
			fmt.Printf("%.0f\t%d\n", b, hist.Get(i))
		}

		return nil
	},
}
