package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/urfave/cli/v2"
)

var listSimCommand = &cli.Command{
	Name: "list",
	Action: func(cctx *cli.Context) error {
		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer node.Close()

		list, err := node.ListSims(cctx.Context)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(cctx.App.Writer, 8, 8, 0, ' ', 0)
		for _, name := range list {
			sim, err := node.LoadSim(cctx.Context, name)
			if err != nil {
				return err
			}
			head := sim.GetHead()
			fmt.Fprintf(tw, "%s\t%s\t%s\n", name, head.Height(), head.Key())
			sim.Close()
		}
		return tw.Flush()
	},
}
