package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/docker/go-units"
	"github.com/fatih/color"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-paramfetch"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/cmd/curio/guidedsetup"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/lib/tracing"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("main")

const (
	FlagMinerRepo = "miner-repo"
)

func setupCloseHandler() {
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

	lotuslog.SetupLogLevels()

	local := []*cli.Command{
		cliCmd,
		runCmd,
		stopCmd,
		configCmd,
		testCmd,
		webCmd,
		guidedsetup.GuidedsetupCmd,
		sealCmd,
		marketCmd,
		fetchParamCmd,
		ffiCmd,
	}

	jaeger := tracing.SetupJaegerTracing("curio")
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
			jaeger = tracing.SetupJaegerTracing("curio/" + cmd.Name)

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
		Name:                 "curio",
		Usage:                "Filecoin decentralized storage network provider",
		Version:              build.UserVersion(),
		EnableBashCompletion: true,
		Before: func(c *cli.Context) error {
			setupCloseHandler()
			return nil
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				// examined in the Before above
				Name:        "color",
				Usage:       "use color in display output",
				DefaultText: "depends on output being a TTY",
			},
			&cli.StringFlag{
				Name:    "panic-reports",
				EnvVars: []string{"CURIO_PANIC_REPORT_PATH"},
				Hidden:  true,
				Value:   "~/.curio", // should follow --repo default
			},
			&cli.StringFlag{
				Name:    "db-host",
				EnvVars: []string{"CURIO_DB_HOST", "CURIO_HARMONYDB_HOSTS"},
				Usage:   "Command separated list of hostnames for yugabyte cluster",
				Value:   "127.0.0.1",
			},
			&cli.StringFlag{
				Name:    "db-name",
				EnvVars: []string{"CURIO_DB_NAME", "CURIO_HARMONYDB_NAME"},
				Value:   "yugabyte",
			},
			&cli.StringFlag{
				Name:    "db-user",
				EnvVars: []string{"CURIO_DB_USER", "CURIO_HARMONYDB_USERNAME"},
				Value:   "yugabyte",
			},
			&cli.StringFlag{
				Name:    "db-password",
				EnvVars: []string{"CURIO_DB_PASSWORD", "CURIO_HARMONYDB_PASSWORD"},
				Value:   "yugabyte",
			},
			&cli.StringFlag{
				Name:    "db-port",
				EnvVars: []string{"CURIO_DB_PORT", "CURIO_HARMONYDB_PORT"},
				Value:   "5433",
			},
			&cli.StringFlag{
				Name:    deps.FlagRepoPath,
				EnvVars: []string{"CURIO_REPO_PATH"},
				Value:   "~/.curio",
			},
			cliutil.FlagVeryVerbose,
		},
		Commands: local,
		After: func(c *cli.Context) error {
			if r := recover(); r != nil {
				p, err := homedir.Expand(c.String(FlagMinerRepo))
				if err != nil {
					log.Errorw("could not expand repo path for panic report", "error", err)
					panic(r)
				}

				// Generate report in CURIO_PATH and re-raise panic
				build.GeneratePanicReport(c.String("panic-reports"), p, c.App.Name)
				panic(r)
			}
			return nil
		},
	}
	app.Setup()
	app.Metadata["repoType"] = repo.Curio
	lcli.RunApp(app)
}

var fetchParamCmd = &cli.Command{
	Name:      "fetch-params",
	Usage:     "Fetch proving parameters",
	ArgsUsage: "[sectorSize]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return xerrors.Errorf("incorrect number of arguments")
		}
		sectorSizeInt, err := units.RAMInBytes(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("error parsing sector size (specify as \"32GiB\", for instance): %w", err)
		}
		sectorSize := uint64(sectorSizeInt)

		err = paramfetch.GetParams(lcli.ReqContext(cctx), build.ParametersJSON(), build.SrsJSON(), sectorSize)
		if err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		return nil
	},
}
