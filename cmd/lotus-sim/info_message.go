package main

import (
	"fmt"
	"syscall"

	"github.com/ipfs/go-cid"
	"github.com/koalacxr/quantile"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation"
	"github.com/filecoin-project/lotus/lib/stati"
)

var infoMessageSizeSimCommand = &cli.Command{
	Name:        "message-size",
	Aliases:     []string{"msg-size"},
	Description: "Output information about message size distribution",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "lookback",
			Value: 0,
		},
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

		go profileOnSignal(cctx, syscall.SIGUSR2)

		sim, err := node.LoadSim(cctx.Context, cctx.String("simulation"))
		if err != nil {
			return err
		}

		qpoints := []struct{ q, tol float64 }{
			{0.30, 0.01},
			{0.40, 0.01},
			{0.60, 0.01},
			{0.70, 0.01},
			{0.80, 0.01},
			{0.85, 0.01},
			{0.90, 0.01},
			{0.95, 0.001},
			{0.99, 0.0005},
			{0.999, 0.0001},
		}
		estims := make([]quantile.Estimate, len(qpoints))
		for i, p := range qpoints {
			estims[i] = quantile.Known(p.q, p.tol)
		}
		qua := quantile.New(estims...)
		hist, err := stati.NewHistogram([]float64{
			1 << 8, 1 << 10, 1 << 11, 1 << 12, 1 << 13, 1 << 14, 1 << 15, 1 << 16,
		})
		if err != nil {
			return err
		}

		err = sim.Walk(cctx.Context, cctx.Int64("lookback"), func(
			sm *stmgr.StateManager, ts *types.TipSet, stCid cid.Cid,
			messages []*simulation.AppliedMessage,
		) error {
			for _, m := range messages {
				msgSize := float64(m.ChainLength())
				qua.Add(msgSize)
				hist.Observe(msgSize)
			}

			return nil
		})
		if err != nil {
			return err
		}
		fmt.Println("Quantiles of message sizes:")
		for _, p := range qpoints {
			fmt.Printf("%.1f%%\t%.0f\n", p.q*100, qua.Get(p.q))
		}
		fmt.Println()
		fmt.Println("Histogram of message sizes:")
		fmt.Printf("Total\t%d\n", hist.Total())
		for i, b := range hist.Buckets[1:] {
			fmt.Printf("%.0f\t%d\t%.1f%%\n", b, hist.Get(i), 100*hist.GetRatio(i))
		}

		return nil
	},
}
