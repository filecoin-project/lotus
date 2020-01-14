package main

import (
	"os"

	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/tracing"
)

var log = logging.Logger("main")

const FlagStorageRepo = "storagerepo"

func main() {
	lotuslog.SetupLogLevels()

	local := []*cli.Command{
		runCmd,
		initCmd,
		infoCmd,
		pledgeSectorCmd,
		sectorsCmd,
	}
	jaeger := tracing.SetupJaegerTracing("lotus")
	defer func() {
		if jaeger != nil {
			jaeger.Flush()
		}
	}()

	for _, cmd := range local {
		cmd := cmd
		originBefore := cmd.Before
		cmd.Before = func(cctx *cli.Context) error {
			trace.UnregisterExporter(jaeger)
			jaeger = tracing.SetupJaegerTracing("lotus/" + cmd.Name)

			if originBefore != nil {
				return originBefore(cctx)
			}
			return nil
		}
	}

	app := &cli.App{
		Name:    "lotus-storage-miner",
		Usage:   "Filecoin decentralized storage network storage miner",
		Version: build.UserVersion,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    FlagStorageRepo,
				EnvVars: []string{"LOTUS_STORAGE_PATH"},
				Value:   "~/.lotusstorage", // TODO: Consider XDG_DATA_HOME
			},
		},

		Commands: append(local, lcli.Commands...),
	}
	app.Setup()
	app.Metadata["repoType"] = repo.StorageMiner

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		os.Exit(1)
	}
}
