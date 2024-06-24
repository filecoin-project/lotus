package main

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation"
)

var infoWindowPostBandwidthSimCommand = &cli.Command{
	Name:        "post-bandwidth",
	Description: "List average chain bandwidth used by window posts for each day of the simulation.",
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

		var postGas, totalGas int64
		printStats := func() {
			_, _ = fmt.Fprintf(cctx.App.Writer, "%.4f%%\n", float64(100*postGas)/float64(totalGas))
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
