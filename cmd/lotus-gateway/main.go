package main

import (
	"context"
	"net"
	"os"

	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats/view"
	"golang.org/x/xerrors"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/gateway"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node"
)

var log = logging.Logger("gateway")

func main() {
	lotuslog.SetupLogLevels()

	local := []*cli.Command{
		runCmd,
	}

	app := &cli.App{
		Name:    "lotus-gateway",
		Usage:   "Public API server for lotus",
		Version: build.UserVersion(),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
		},

		Commands: local,
	}
	app.Setup()

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		return
	}
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start api server",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Usage: "host address and port the api server will listen on",
			Value: "0.0.0.0:2346",
		},
		&cli.IntFlag{
			Name:  "api-max-req-size",
			Usage: "maximum API request size accepted by the JSON RPC server",
		},
		&cli.DurationFlag{
			Name:  "api-max-lookback",
			Usage: "maximum duration allowable for tipset lookbacks",
			Value: gateway.DefaultLookbackCap,
		},
		&cli.Int64Flag{
			Name:  "api-wait-lookback-limit",
			Usage: "maximum number of blocks to search back through for message inclusion",
			Value: int64(gateway.DefaultStateWaitLookbackLimit),
		},
	},
	Action: func(cctx *cli.Context) error {
		log.Info("Starting lotus gateway")

		ctx := lcli.ReqContext(cctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Register all metric views
		if err := view.Register(
			metrics.ChainNodeViews...,
		); err != nil {
			log.Fatalf("Cannot register the view: %v", err)
		}

		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()

		var (
			lookbackCap  = cctx.Duration("api-max-lookback")
			address      = cctx.String("listen")
			waitLookback = abi.ChainEpoch(cctx.Int64("api-wait-lookback-limit"))
		)

		serverOptions := make([]jsonrpc.ServerOption, 0)
		if maxRequestSize := cctx.Int("api-max-req-size"); maxRequestSize != 0 {
			serverOptions = append(serverOptions, jsonrpc.WithMaxRequestSize(int64(maxRequestSize)))
		}

		log.Info("setting up API endpoint at " + address)

		addr, err := net.ResolveTCPAddr("tcp", address)
		if err != nil {
			return xerrors.Errorf("failed to resolve endpoint address: %w", err)
		}

		maddr, err := manet.FromNetAddr(addr)
		if err != nil {
			return xerrors.Errorf("failed to convert endpoint address to multiaddr: %w", err)
		}

		gwapi := gateway.NewNode(api, lookbackCap, waitLookback)
		h, err := gateway.Handler(gwapi, serverOptions...)
		if err != nil {
			return xerrors.Errorf("failed to set up gateway HTTP handler")
		}

		stopFunc, err := node.ServeRPC(h, "lotus-gateway", maddr)
		if err != nil {
			return xerrors.Errorf("failed to serve rpc endpoint: %w", err)
		}

		<-node.MonitorShutdown(nil, node.ShutdownHandler{
			Component: "rpc",
			StopFunc:  stopFunc,
		})
		return nil
	},
}
