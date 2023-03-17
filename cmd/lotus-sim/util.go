package main

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/brossetti1/lotus/cmd/lotus-sim/simulation"
	"github.com/brossetti1/lotus/lib/ulimit"
)

func open(cctx *cli.Context) (*simulation.Node, error) {
	_, _, err := ulimit.ManageFdLimit()
	if err != nil {
		fmt.Fprintf(cctx.App.ErrWriter, "ERROR: failed to raise ulimit: %s\n", err)
	}
	return simulation.OpenNode(cctx.Context, cctx.String("repo"))
}
