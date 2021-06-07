package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/urfave/cli/v2"
)

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
		tw := tabwriter.NewWriter(cctx.App.Writer, 8, 8, 0, ' ', 0)
		fmt.Fprintln(tw, "Name:\t", sim.Name())
		fmt.Fprintln(tw, "Height:\t", sim.GetHead().Height())
		fmt.Fprintln(tw, "TipSet:\t", sim.GetHead())
		fmt.Fprintln(tw, "Network Version:\t", sim.GetNetworkVersion())
		return tw.Flush()
	},
}
