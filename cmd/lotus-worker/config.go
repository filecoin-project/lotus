package main

import (
	"fmt"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/urfave/cli/v2"
)

var configCmd = &cli.Command{
	Name:  "config",
	Usage: "Manage node config",
	Subcommands: []*cli.Command{
		configDefaultCmd,
		// configUpdateCmd,
	},
}

var configDefaultCmd = &cli.Command{
	Name:  "default",
	Usage: "Print default node config",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-comment",
			Usage: "don't comment default values",
		},
	},
	Action: func(cctx *cli.Context) error {
		c := config.DefaultStorageWorker()

		cb, err := config.ConfigUpdate(c, nil, !cctx.Bool("no-comment"))
		if err != nil {
			return err
		}

		fmt.Println(string(cb))

		return nil
	},
}
