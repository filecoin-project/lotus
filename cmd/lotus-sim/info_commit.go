package main

import (
	"bytes"
	"fmt"
	"os"
	"syscall"

	"github.com/ipfs/go-cid"
	"github.com/koalacxr/quantile"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation"
	"github.com/filecoin-project/lotus/lib/stati"
)

var infoCommitGasSimCommand = &cli.Command{
	Name:        "commit-gas",
	Description: "Output information about the gas for commits",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "lookback",
			Value: 0,
		},
	},
	Action: func(cctx *cli.Context) (err error) {
		log := func(f string, i ...interface{}) {
			fmt.Fprintf(os.Stderr, f, i...)
		}
		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := node.Close(); err == nil {
				err = cerr
			}
		}()

		go profileOnSignal(cctx, syscall.SIGUSR2)

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
					param := minertypes.ProveCommitAggregateParams{}
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
