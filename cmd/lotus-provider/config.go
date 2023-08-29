package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var configCmd = &cli.Command{
	Name:  "config",
	Usage: "Manage node config",
	Subcommands: []*cli.Command{
		configDefaultCmd,
		configSetCmd,
		configGetCmd,
	},
}

var configDefaultCmd = &cli.Command{
	Name:  "default",
	Usage: "Print default system config",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-comment",
			Usage: "don't comment default values",
		},
	},
	Action: func(cctx *cli.Context) error {
		fmt.Println("[config]\nstatus = Coming Soon")
		// [overlay.sealer1.tasks]\nsealer_task_enable = true
		return nil
	},
}

var configSetCmd = &cli.Command{
	Name:  "set",
	Usage: "Set all config",
	Action: func(cctx *cli.Context) error {
		fmt.Println("Coming soon")
		return nil
	},
}

var configGetCmd = &cli.Command{
	Name:  "get",
	Usage: "Get all config",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-comment",
			Usage: "don't comment default values",
		},
		&cli.BoolFlag{
			Name:  "no-doc",
			Usage: "don't add value documentation",
		},
	},
	Action: func(cctx *cli.Context) error {
		fmt.Println("Coming soon")
		return nil
	},
}
