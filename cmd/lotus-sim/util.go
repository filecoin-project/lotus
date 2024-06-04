package main

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation"
	"github.com/filecoin-project/lotus/lib/ulimit"
)

func open(cctx *cli.Context) (*simulation.Node, error) {
	if _, _, err := ulimit.ManageFdLimit(); err != nil {
		_, _ = fmt.Fprintf(cctx.App.ErrWriter, "ERROR: failed to raise ulimit: %s\n", err)
	}
	return simulation.OpenNode(cctx.Context, cctx.String("repo"))
}
