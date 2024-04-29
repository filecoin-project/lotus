package main

import (
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/cli/spcli"
)

var provingCmd = &cli.Command{
	Name:  "proving",
	Usage: "View proving information",
	Subcommands: []*cli.Command{
		spcli.ProvingInfoCmd(SPTActorGetter),
		spcli.ProvingDeadlinesCmd(SPTActorGetter),
		spcli.ProvingDeadlineInfoCmd(SPTActorGetter),
		spcli.ProvingFaultsCmd(SPTActorGetter),
	},
}
