package main

import (
	"context"
	"os"

	logging "github.com/ipfs/go-log"
	"go.opencensus.io/trace"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/tracing"
)

func main() {
	logging.SetLogLevel("*", "INFO")
	logging.SetLogLevel("dht", "ERROR")
	logging.SetLogLevel("swarm2", "WARN")
	logging.SetLogLevel("bitswap", "WARN")
	logging.SetLogLevel("pubsub", "WARN")
	logging.SetLogLevel("connmgr", "WARN")

	local := []*cli.Command{
		DaemonCmd,
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
	ctx, span := trace.StartSpan(context.Background(), "/cli")
	defer span.End()

	app := &cli.App{
		Name:    "lotus",
		Usage:   "Filecoin decentralized storage network client",
		Version: build.UserVersion,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
		},

		Commands: append(local, lcli.Commands...),
	}
	app.Setup()
	app.Metadata["traceContext"] = ctx
	app.Metadata["repoType"] = repo.FullNode

	if err := app.Run(os.Args); err != nil {
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeFailedPrecondition,
			Message: err.Error(),
		})
		log.Warnf("%+v", err)
		os.Exit(1)
	}
}
