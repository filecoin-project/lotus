package main

import (
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
)

var upgradeCommand = &cli.Command{
	Name:        "upgrade",
	Description: "Modifies network upgrade heights.",
	Subcommands: []*cli.Command{
		upgradeSetCommand,
		upgradeList,
	},
}

var upgradeList = &cli.Command{
	Name:        "list",
	Description: "Lists all pending upgrades.",
	Subcommands: []*cli.Command{
		upgradeSetCommand,
	},
	Action: func(cctx *cli.Context) (err error) {
		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := node.Close(); err == nil {
				err = cerr
			}
		}()

		sim, err := node.LoadSim(cctx.Context, cctx.String("simulation"))
		if err != nil {
			return err
		}
		upgrades, err := sim.ListUpgrades()
		if err != nil {
			return err
		}

		tw := tabwriter.NewWriter(cctx.App.Writer, 8, 8, 0, ' ', 0)
		_, _ = fmt.Fprintf(tw, "version\theight\tepochs\tmigration\texpensive")
		epoch := sim.GetHead().Height()
		for _, upgrade := range upgrades {
			_, _ = fmt.Fprintf(
				tw, "%d\t%d\t%+d\t%t\t%t",
				upgrade.Network, upgrade.Height, upgrade.Height-epoch,
				upgrade.Migration != nil,
				upgrade.Expensive,
			)
		}
		return nil
	},
}

var upgradeSetCommand = &cli.Command{
	Name:        "set",
	ArgsUsage:   "<network-version> [+]<epochs>",
	Description: "Set a network upgrade height. Prefix with '+' to set it relative to the last epoch.",
	Action: func(cctx *cli.Context) (err error) {
		args := cctx.Args()
		if args.Len() != 2 {
			return fmt.Errorf("expected 2 arguments")
		}
		nvString := args.Get(0)
		networkVersion, err := strconv.ParseUint(nvString, 10, 32)
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
		defer func() {
			if cerr := node.Close(); err == nil {
				err = cerr
			}
		}()

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
