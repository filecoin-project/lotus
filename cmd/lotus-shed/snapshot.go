package main

import (
	"github.com/urfave/cli/v2"
)

var snapshotCmd = &cli.Command{
	Name:        "snapshot",
	Description: "interact with filecoin chain snapshots",
	Subcommands: []*cli.Command{
		snapshotStatCmd,
	},
}

var snapshotStatCmd = &cli.Command{
	Name:      "stat-actor",
	Usage:     "display number of snapshot bytes held by provided actor",
	ArgsUsage: "[datastore path] [head] [actor address]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "field", // specify top level actor field to stat
		},
	},
	Action: func(cctx *cli.Context) error {
		// Initialize in memory graph counter
		// Get root tskey
		// Walk headers back

		// 		Count header bytes
		//      open state,
		//      if no field set graph count actor HEAD
		// 		if field is set, parse actor head, look for field
		//      if field not found or not a cid error, otherwise do graph count on the cid

		// Print out stats

		return nil
	},
}
