package main

import "github.com/urfave/cli/v2"

var actorCmd = &cli.Command{
	Name:        "actor",
	Usage:       "Manage Filecoin Miner Actor Metadata",
	Subcommands: []*cli.Command{},
}
