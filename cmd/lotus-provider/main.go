package main

import (
	"context"

	"github.com/fatih/color"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/lib/tracing"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("main")

func main() {

	lotuslog.SetupLogLevels()

	local := []*cli.Command{
		//initCmd,
		runCmd,
		stopCmd,
		//configCmd,
		//backupCmd,
		//lcli.WithCategory("chain", actorCmd),
		//lcli.WithCategory("storage", sectorsCmd),
		//lcli.WithCategory("storage", provingCmd),
		//lcli.WithCategory("storage", storageCmd),
		//lcli.WithCategory("storage", sealingCmd),
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

	app := &cli.App{
		Name:                 "lotus-provider",
		Usage:                "Filecoin decentralized storage network provider",
		Version:              build.UserVersion(),
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "listen",
				Usage:   "host address and port the worker api will listen on",
				Value:   "0.0.0.0:3456",
				EnvVars: []string{"LOTUS_WORKER_LISTEN"},
			},
			&cli.BoolFlag{
				// examined in the Before above
				Name:        "color",
				Usage:       "use color in display output",
				DefaultText: "depends on output being a TTY",
			},
			&cli.StringFlag{
				Name:    "panic-reports",
				EnvVars: []string{"LOTUS_PANIC_REPORT_PATH"},
				Hidden:  true,
				Value:   "~/.lotusprovider", // should follow --repo default
			},
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
			cliutil.FlagVeryVerbose,
		},
		Commands: append(local, lcli.CommonCommands...),
		Before: func(c *cli.Context) error {
			return nil
		},
		After: func(c *cli.Context) error {
			if r := recover(); r != nil {
				// Generate report in LOTUS_PATH and re-raise panic
				build.GeneratePanicReport(c.String("panic-reports"), c.String(FlagProviderRepo), c.App.Name)
				panic(r)
			}
			return nil
		},
	}
	app.Setup()
	app.Metadata["repoType"] = repo.Provider
	lcli.RunApp(app)
}

const (
	FlagProviderRepo = "provider-repo"
)
