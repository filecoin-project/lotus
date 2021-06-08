package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var renameSimCommand = &cli.Command{
	Name:      "rename",
	ArgsUsage: "<new-name>",
	Action: func(cctx *cli.Context) error {
		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer node.Close()
		if cctx.NArg() != 1 {
			return fmt.Errorf("expected 1 argument")
		}
		name := cctx.Args().First()
		return node.RenameSim(cctx.Context, cctx.String("simulation"), name)
	},
}
