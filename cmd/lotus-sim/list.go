package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/urfave/cli/v2"
)

var listSimCommand = &cli.Command{
	Name: "list",
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
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", name, head.Height(), head.Key())
		}
		return tw.Flush()
	},
}
