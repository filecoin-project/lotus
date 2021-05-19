package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/urfave/cli/v2"
)

var setUpgradeCommand = &cli.Command{
	Name:        "set-upgrade",
	ArgsUsage:   "<network-version> [+]<epochs>",
	Description: "Set a network upgrade height. prefix with '+' to set it relative to the last epoch.",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() != 2 {
			return fmt.Errorf("expected 2 arguments")
		}
		nvString := args.Get(0)
		networkVersion, err := strconv.ParseInt(nvString, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse network version %q: %w", nvString, err)
		}
		heightString := args.Get(1)
		relative := false
		if strings.HasPrefix(heightString, "+") {
			heightString = heightString[1:]
			relative = true
		}
		height, err := strconv.ParseInt(heightString, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse height version %q: %w", heightString, err)
		}

		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer node.Close()

		sim, err := node.LoadSim(cctx.Context, cctx.String("simulation"))
		if err != nil {
			return err
		}
		if relative {
			height += int64(sim.GetHead().Height())
		}
		return sim.SetUpgradeHeight(network.Version(networkVersion), abi.ChainEpoch(height))
	},
}
