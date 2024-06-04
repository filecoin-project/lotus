package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"
)

var runSimCommand = &cli.Command{
	Name: "run",
	Description: `Run the simulation.

Signals:
- SIGUSR1: Print information about the current simulation (equivalent to 'lotus-sim info').
- SIGUSR2: Write pprof profiles to ./pprof-simulation-$DATE/`,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "epochs",
			Usage: "Advance the given number of epochs then stop.",
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
		targetEpochs := cctx.Int("epochs")

		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGUSR1)
		defer signal.Stop(ch)

		for i := 0; targetEpochs == 0 || i < targetEpochs; i++ {
			ts, err := sim.Step(cctx.Context)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cctx.App.Writer, "advanced to %d %s\n", ts.Height(), ts.Key())

			// Print
			select {
			case <-ch:
				_, _ = fmt.Fprintln(cctx.App.Writer, "---------------------")
				if err := printInfo(cctx.Context, sim, cctx.App.Writer); err != nil {
					_, _ = fmt.Fprintf(cctx.App.ErrWriter, "ERROR: failed to print info: %s\n", err)
				}
				_, _ = fmt.Fprintln(cctx.App.Writer, "---------------------")
			case <-cctx.Context.Done():
				return cctx.Err()
			default:
			}
		}
		_, _ = fmt.Fprintln(cctx.App.Writer, "simulation done")
		return err
	},
}
