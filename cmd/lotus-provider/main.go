package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/fatih/color"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/lib/tracing"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("main")

func SetupCloseHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\r- Ctrl+C pressed in Terminal")
		_ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		panic(1)
	}()
}

func main() {
	SetupCloseHandler()

	lotuslog.SetupLogLevels()

	local := []*cli.Command{
		//initCmd,
		cliCmd,
		runCmd,
		stopCmd,
		configCmd,
		testCmd,
		webCmd,
		sealCmd,
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
				Name:    "db-host",
				EnvVars: []string{"LOTUS_DB_HOST"},
				Usage:   "Command separated list of hostnames for yugabyte cluster",
				Value:   "yugabyte",
			},
			&cli.StringFlag{
				Name:    "db-name",
				EnvVars: []string{"LOTUS_DB_NAME", "LOTUS_HARMONYDB_HOSTS"},
				Value:   "yugabyte",
			},
			&cli.StringFlag{
				Name:    "db-user",
				EnvVars: []string{"LOTUS_DB_USER", "LOTUS_HARMONYDB_USERNAME"},
				Value:   "yugabyte",
			},
			&cli.StringFlag{
				Name:    "db-password",
				EnvVars: []string{"LOTUS_DB_PASSWORD", "LOTUS_HARMONYDB_PASSWORD"},
				Value:   "yugabyte",
			},
			&cli.StringFlag{
				Name:    "db-port",
				EnvVars: []string{"LOTUS_DB_PORT", "LOTUS_HARMONYDB_PORT"},
				Hidden:  true,
				Value:   "5433",
			},
			&cli.StringFlag{
				Name:    deps.FlagRepoPath,
				EnvVars: []string{"LOTUS_REPO_PATH"},
				Value:   "~/.lotusprovider",
			},
			cliutil.FlagVeryVerbose,
		},
		Commands: append(local, lcli.CommonCommands...),
		Before: func(c *cli.Context) error {
			return nil
		},
		After: func(c *cli.Context) error {
			if r := recover(); r != nil {
				p, err := homedir.Expand(c.String(FlagMinerRepo))
				if err != nil {
					log.Errorw("could not expand repo path for panic report", "error", err)
					panic(r)
				}

				// Generate report in LOTUS_PATH and re-raise panic
				build.GeneratePanicReport(c.String("panic-reports"), p, c.App.Name)
				panic(r)
			}
			return nil
		},
	}
	app.Setup()
	app.Metadata["repoType"] = repo.Provider
	lcli.RunApp(app)
}
