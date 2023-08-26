package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/ulimit"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start a lotus provider process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "provider-api",
			Usage: "Port (default 12300)",
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
			Name:    "db-host",
			EnvVars: []string{"LOTUS_DB_HOST"},
			Usage:   "Command separated list of hostnames for yugabyte cluster",
			Value:   "yugabyte",
		},
		&cli.StringFlag{
			Name:    "db-name",
			EnvVars: []string{"LOTUS_DB_NAME"},
			Value:   "yugabyte",
		},
		&cli.StringFlag{
			Name:    "db-user",
			EnvVars: []string{"LOTUS_DB_USER"},
			Value:   "yugabyte",
		},
		&cli.StringFlag{
			Name:    "db-password",
			EnvVars: []string{"LOTUS_DB_PASSWORD"},
			Value:   "yugabyte",
		},
		&cli.StringFlag{
			Name:    "db-port",
			EnvVars: []string{"LOTUS_DB_PORT"},
			Hidden:  true,
			Value:   "5433",
		},
	},
	Action: func(cctx *cli.Context) error {
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

		// Open repo

		repoPath := cctx.String(FlagProviderRepo)
		r, err := repo.NewFS(repoPath)
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}
		if !ok {
			if err := r.Init(repo.Provider); err != nil {
				return err
			}

			lr, err := r.Lock(repo.Provider)
			if err != nil {
				return err
			}

			var localPaths []storiface.LocalPath

			if err := lr.SetStorage(func(sc *storiface.StorageConfig) {
				sc.StoragePaths = append(sc.StoragePaths, localPaths...)
			}); err != nil {
				return fmt.Errorf("set storage config: %w", err)
			}

			{
				// init datastore for r.Exists
				_, err := lr.Datastore(context.Background(), "/metadata")
				if err != nil {
					return err
				}
			}
			if err := lr.Close(); err != nil {
				return fmt.Errorf("close repo: %w", err)
			}
		}

		lr, err := r.Lock(repo.Provider)
		if err != nil {
			return err
		}
		defer func() {
			if err := lr.Close(); err != nil {
				log.Error("closing repo", err)
			}
		}()

		db, err := harmonydb.NewFromConfig(config.HarmonyDB{
			Username: cctx.String("db_user"),
			Password: cctx.String("db_password"),
			Hosts:    strings.Split(cctx.String("db_host"), ","),
			Database: cctx.String("db_name"),
			Port:     cctx.String("db_port"),
		})
		if err != nil {
			return err
		}

		shutdownChan := make(chan struct{})

		stop, err := node.New(ctx,
			node.Override(new(dtypes.ShutdownChan), shutdownChan),
			node.Provider(r),
		)
		if err != nil {
			return fmt.Errorf("creating node: %w", err)
		}

		const unspecifiedAddress = "0.0.0.0"
		address := cctx.String("listen")
		addressSlice := strings.Split(address, ":")
		if ip := net.ParseIP(addressSlice[0]); ip != nil {
			if ip.String() == unspecifiedAddress {
				rip, err := db.GetRoutableIP()
				if err != nil {
					return err
				}
				address = rip + ":" + addressSlice[1]
			}
		}
		localStore, err := paths.NewLocal(ctx, lr, nil, []string{"http://" + address + "/remote"})
		if err != nil {
			return err
		}

		taskEngine, err := harmonytask.New(db, []harmonytask.TaskInterface{}, address)
		if err != nil {
			return err
		}
		handler := gin.New()

		taskEngine.ApplyHttpHandlers(handler.Group("/"))
		defer taskEngine.GracefullyTerminate(time.Hour)

		fh := &paths.FetchHandler{Local: localStore, PfHandler: &paths.DefaultPartialFileHandler{}}
		handler.NoRoute(gin.HandlerFunc(func(c *gin.Context) {
			if !auth.HasPerm(c, nil, api.PermAdmin) {
				c.JSON(401, struct{ Error string }{"unauthorized: missing admin permission"})
				return
			}

			fh.ServeHTTP(c.Writer, c.Request)
		}))
		// local APIs
		{
			// debugging
			handler.GET("/debug/metrics", gin.WrapH(metrics.Exporter()))
			pprof.Register(handler)
		}

		// Serve the RPC.
		endpoint, err := r.APIEndpoint()
		if err != nil {
			return fmt.Errorf("getting API endpoint: %w", err)
		}
		rpcStopper, err := node.ServeRPC(handler, "lotus-provider", endpoint)
		if err != nil {
			return fmt.Errorf("failed to start json-rpc endpoint: %s", err)
		}

		// Monitor for shutdown.
		finishCh := node.MonitorShutdown(shutdownChan,
			node.ShutdownHandler{Component: "rpc server", StopFunc: rpcStopper},
			node.ShutdownHandler{Component: "provider", StopFunc: stop},
		)

		<-finishCh
		return nil
	},
}
