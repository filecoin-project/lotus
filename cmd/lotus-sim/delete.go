package main

import (
	"github.com/urfave/cli/v2"
)

var deleteSimCommand = &cli.Command{
	Name: "delete",
	Action: func(cctx *cli.Context) error {
		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer node.Close()

		return node.DeleteSim(cctx.Context, cctx.String("simulation"))
	},
}
