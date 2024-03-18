package main

import (
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/cli/spcli"
)

var actorCmd = &cli.Command{
	Name:  "actor",
	Usage: "Manage Filecoin Miner Actor Metadata",
	Subcommands: []*cli.Command{
		spcli.ActorWithdrawCmd(SPTActorGetter),
	},
}
