package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var runSimCommand = &cli.Command{
	Name:        "run",
	Description: "Run the simulation.",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "epochs",
			Usage: "Advance the given number of epochs then stop.",
		},
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
		fmt.Fprintln(cctx.App.Writer, "loading simulation")
		err = sim.Load(cctx.Context)
		if err != nil {
			return err
		}
		fmt.Fprintln(cctx.App.Writer, "running simulation")
		targetEpochs := cctx.Int("epochs")
		for i := 0; targetEpochs == 0 || i < targetEpochs; i++ {
			ts, err := sim.Step(cctx.Context)
			if err != nil {
				return err
			}
			fmt.Fprintf(cctx.App.Writer, "advanced to %d %s\n", ts.Height(), ts.Key())
		}
		fmt.Fprintln(cctx.App.Writer, "simulation done")
		return err
	},
}
