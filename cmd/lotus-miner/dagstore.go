package main

import (
	"github.com/urfave/cli/v2"
)

var dagstoreCmd = &cli.Command{
	Name:  "dagstore",
	Usage: "Manage the DAG store",
	Subcommands: []*cli.Command{
		dagstoreListShardsCmd,
		dagstoreGarbageCollectCmd,
	},
}

var dagstoreListShardsCmd = &cli.Command{
	Name:  "list-shards",
	Usage: "List shards known to the DAG store",
	Action: func(cctx *cli.Context) error {
		return nil
	},
}

var dagstoreGarbageCollectCmd = &cli.Command{
	Name:  "gc",
	Usage: "Garbage collect the DAG store",
	Action: func(cctx *cli.Context) error {
		return nil
	},
}
