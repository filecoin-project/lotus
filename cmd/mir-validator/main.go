package main

import (
	"context"

	"github.com/fatih/color"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/lib/tracing"
)

var log = logging.Logger("mir-validator-cli")

func main() {
	api.RunningNodeType = api.NodeMiner

	lotuslog.SetupLogLevels()

	local := []*cli.Command{
		runCmd,
		cfgCmd,
	}

	jaeger := tracing.SetupJaegerTracing("lotus")
	defer func() {
		if jaeger != nil {
			_ = jaeger.ForceFlush(context.Background())
		}
	}()

	for _, cmd := range local {
		cmd := cmd
		originBefore := cmd.Before
		cmd.Before = func(cctx *cli.Context) error {
			if jaeger != nil {
				_ = jaeger.Shutdown(cctx.Context)
			}
			jaeger = tracing.SetupJaegerTracing("lotus/" + cmd.Name)

			if cctx.IsSet("color") {
				color.NoColor = !cctx.Bool("color")
			}

			if originBefore != nil {
				return originBefore(cctx)
			}

			return nil
		}
	}

	// TODO: Enable NET API for mir validator libp2p host?
	// // adapt the Net* commands to always hit the node running the markets
	// // subsystem, as that is the only one that runs a libp2p node.
	// netCmd := *lcli.NetCmd // make a copy.
	// prev := netCmd.Before
	// netCmd.Before = func(c *cli.Context) error {
	// 	if prev != nil {
	// 		if err := prev(c); err != nil {
	// 			return err
	// 		}
	// 	}
	// 	return nil
	// }

	app := &cli.App{
		Name:                 "mir-validator",
		Usage:                "Mir validator implementation for Filecoin",
		Version:              build.UserVersion(),
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				// examined in the Before above
				Name:        "color",
				Usage:       "use color in display output",
				DefaultText: "depends on output being a TTY",
			},
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
			cliutil.FlagVeryVerbose,
		},
		Commands: append(local, append(lcli.CommonCommands /*, &netCmd*/)...),
	}
	app.Setup()
	lcli.RunApp(app)
}
