package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/rpc"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/tasks"
	"github.com/filecoin-project/lotus/lib/ulimit"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
)

type stackTracer interface {
	StackTrace() errors.StackTrace
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start a lotus provider process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "listen",
			Usage:   "host address and port the worker api will listen on",
			Value:   "0.0.0.0:12300",
			EnvVars: []string{"LOTUS_WORKER_LISTEN"},
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
		&cli.StringSliceFlag{
			Name:  "layers",
			Usage: "list of layers to be interpreted (atop defaults). Default: base",
			Value: cli.NewStringSlice("base"),
		},
		&cli.StringFlag{
			Name:  "storage-json",
			Usage: "path to json file containing storage config",
			Value: "~/.lotus-provider/storage.json",
		},
		&cli.StringFlag{
			Name:  "journal",
			Usage: "path to journal files",
			Value: "~/.lotus-provider/",
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

		ctx, _ := tag.New(lcli.DaemonContext(cctx),
			tag.Insert(metrics.Version, build.BuildVersion),
			tag.Insert(metrics.Commit, build.CurrentCommit),
			tag.Insert(metrics.NodeType, "provider"),
		)
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
			fmt.Println("err", err)
			return err
		}
		fmt.Println("ef")

		taskEngine, err := tasks.StartTasks(ctx, dependencies)
		fmt.Println("gh")

		if err != nil {
			return nil
		}
		defer taskEngine.GracefullyTerminate(time.Hour)

		err = rpc.ListenAndServe(ctx, dependencies, shutdownChan) // Monitor for shutdown.
		if err != nil {
			return err
		}
		finishCh := node.MonitorShutdown(shutdownChan) //node.ShutdownHandler{Component: "rpc server", StopFunc: rpcStopper},
		//node.ShutdownHandler{Component: "provider", StopFunc: stop},

		<-finishCh
		return nil
	},
}

var webCmd = &cli.Command{
	Name:  "web",
	Usage: "Start lotus provider web interface",
	Description: `Start an instance of lotus provider web interface. 
	This creates the 'web' layer if it does not exist, then calls run with that layer.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Usage: "Address to listen on",
			Value: "127.0.0.1:4701",
		},
	},
	Action: func(cctx *cli.Context) error {
		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}

		webtxt, err := getConfig(db, "web")
		if err != nil || webtxt == "" {
			cfg := config.DefaultLotusProvider()
			cfg.Subsystems.EnableWebGui = true
			var b bytes.Buffer
			toml.NewEncoder(&b).Encode(cfg)
			if err = setConfig(db, "web", b.String()); err != nil {
				return err
			}
		}
		cctx.Set("layers", "web")
		return runCmd.Action(cctx)
	},
}
