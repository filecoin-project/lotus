package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/cmd/curio/rpc"
	"github.com/filecoin-project/lotus/cmd/curio/tasks"
	"github.com/filecoin-project/lotus/curiosrc/market/lmrpc"
	"github.com/filecoin-project/lotus/lib/ulimit"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node"
)

type stackTracer interface {
	StackTrace() errors.StackTrace
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start a Curio process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "listen",
			Usage:   "host address and port the worker api will listen on",
			Value:   "0.0.0.0:12300",
			EnvVars: []string{"CURIO_LISTEN"},
		},
		&cli.StringFlag{
			Name:   "gui-listen",
			Usage:  "host address and port the gui will listen on",
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:  "nosync",
			Usage: "don't check full-node sync status",
		},
		&cli.BoolFlag{
			Name:   "halt-after-init",
			Usage:  "only run init, then return",
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:  "manage-fdlimit",
			Usage: "manage open file limit",
			Value: true,
		},
		&cli.StringFlag{
			Name:  "storage-json",
			Usage: "path to json file containing storage config",
			Value: "~/.curio/storage.json",
		},
		&cli.StringFlag{
			Name:  "journal",
			Usage: "path to journal files",
			Value: "~/.curio/",
		},
		&cli.StringSliceFlag{
			Name:    "layers",
			Usage:   "list of layers to be interpreted (atop defaults). Default: base",
			EnvVars: []string{"CURIO_LAYERS"},
			Aliases: []string{"l", "layer"},
		},
	},
	Action: func(cctx *cli.Context) (err error) {
		defer func() {
			if err != nil {
				if err, ok := err.(stackTracer); ok {
					for _, f := range err.StackTrace() {
						fmt.Printf("%+s:%d\n", f, f)
					}
				}
			}
		}()
		if !cctx.Bool("enable-gpu-proving") {
			err := os.Setenv("BELLMAN_NO_GPU", "true")
			if err != nil {
				return err
			}
		}

		if err := os.MkdirAll(os.TempDir(), 0755); err != nil {
			log.Errorf("ensuring tempdir exists: %s", err)
		}

		ctx := lcli.DaemonContext(cctx)
		shutdownChan := make(chan struct{})
		{
			var ctxclose func()
			ctx, ctxclose = context.WithCancel(ctx)
			go func() {
				<-shutdownChan
				ctxclose()
			}()
		}
		// Register all metric views
		/*
			if err := view.Register(
				metrics.MinerNodeViews...,
			); err != nil {
				log.Fatalf("Cannot register the view: %v", err)
			}
		*/
		// Set the metric to one so it is published to the exporter
		stats.Record(ctx, metrics.LotusInfo.M(1))

		if cctx.Bool("manage-fdlimit") {
			if _, _, err := ulimit.ManageFdLimit(); err != nil {
				log.Errorf("setting file descriptor limit: %s", err)
			}
		}

		dependencies := &deps.Deps{}
		err = dependencies.PopulateRemainingDeps(ctx, cctx, true)
		if err != nil {
			return err
		}

		go ffiSelfTest() // Panics on failure

		taskEngine, err := tasks.StartTasks(ctx, dependencies)

		if err != nil {
			return nil
		}
		defer taskEngine.GracefullyTerminate()

		if err := lmrpc.ServeCurioMarketRPCFromConfig(dependencies.DB, dependencies.Full, dependencies.Cfg); err != nil {
			return xerrors.Errorf("starting market RPCs: %w", err)
		}

		err = rpc.ListenAndServe(ctx, dependencies, shutdownChan) // Monitor for shutdown.
		if err != nil {
			return err
		}

		finishCh := node.MonitorShutdown(shutdownChan) //node.ShutdownHandler{Component: "rpc server", StopFunc: rpcStopper},
		//node.ShutdownHandler{Component: "curio", StopFunc: stop},

		<-finishCh
		return nil
	},
}

var layersFlag = &cli.StringSliceFlag{
	Name:  "layers",
	Usage: "list of layers to be interpreted (atop defaults). Default: base",
}

var webCmd = &cli.Command{
	Name:  "web",
	Usage: "Start Curio web interface",
	Description: `Start an instance of Curio web interface. 
	This creates the 'web' layer if it does not exist, then calls run with that layer.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "gui-listen",
			Usage: "Address to listen for the GUI on",
			Value: "0.0.0.0:4701",
		},
		&cli.BoolFlag{
			Name:  "nosync",
			Usage: "don't check full-node sync status",
		},
		layersFlag,
	},
	Action: func(cctx *cli.Context) error {

		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}

		webtxt, err := getConfig(db, "web")
		if err != nil || webtxt == "" {

			s := `[Susbystems]
			EnableWebGui = true
			`
			if err = setConfig(db, "web", s); err != nil {
				return err
			}
		}
		layers := append([]string{"web"}, cctx.StringSlice("layers")...)
		err = cctx.Set("layers", strings.Join(layers, ","))
		if err != nil {
			return err
		}
		return runCmd.Action(cctx)
	},
}
