package main

import (
	"github.com/urfave/cli/v2"
)

var deleteSimCommand = &cli.Command{
	Name: "delete",
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

		return node.DeleteSim(cctx.Context, cctx.String("simulation"))
	},
}
