package main

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/tools/stats/influx"
	"github.com/filecoin-project/lotus/tools/stats/ipldstore"
	"github.com/filecoin-project/lotus/tools/stats/metrics"
	"github.com/filecoin-project/lotus/tools/stats/points"
	"github.com/filecoin-project/lotus/tools/stats/sync"
)

var log = logging.Logger("stats")

func init() {
	if err := view.Register(metrics.DefaultViews...); err != nil {
		log.Fatal(err)
	}
}

func main() {
	local := []*cli.Command{
		runCmd,
		versionCmd,
	}

	app := &cli.App{
		Name:    "lotus-stats",
		Usage:   "Collect basic information about a filecoin network using lotus",
		Version: string(build.NodeUserVersion()),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "lotus-path",
				EnvVars: []string{"LOTUS_PATH"},
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    "log-level",
				EnvVars: []string{"LOTUS_STATS_LOG_LEVEL"},
				Value:   "info",
			},
		},
		Before: func(cctx *cli.Context) error {
			return logging.SetLogLevelRegex("stats/*", cctx.String("log-level"))
		},
		Commands: local,
	}

	if err := app.Run(os.Args); err != nil {
		log.Errorw("exit in error", "err", err)
		os.Exit(1)
		return
	}
}

var versionCmd = &cli.Command{
	Name:  "version",
	Usage: "Print version",
	Action: func(cctx *cli.Context) error {
		cli.VersionPrinter(cctx)
		return nil
	},
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "influx-database",
			EnvVars: []string{"LOTUS_STATS_INFLUX_DATABASE"},
			Usage:   "influx database",
			Value:   "",
		},
		&cli.StringFlag{
			Name:    "influx-hostname",
			EnvVars: []string{"LOTUS_STATS_INFLUX_HOSTNAME"},
			Value:   "http://localhost:8086",
			Usage:   "influx hostname",
		},
		&cli.StringFlag{
			Name:    "influx-username",
			EnvVars: []string{"LOTUS_STATS_INFLUX_USERNAME"},
			Usage:   "influx username",
			Value:   "",
		},
		&cli.StringFlag{
			Name:    "influx-password",
			EnvVars: []string{"LOTUS_STATS_INFLUX_PASSWORD"},
			Usage:   "influx password",
			Value:   "",
		},
		&cli.IntFlag{
			Name:    "height",
			EnvVars: []string{"LOTUS_STATS_HEIGHT"},
			Usage:   "tipset height to start processing from",
			Value:   0,
		},
		&cli.IntFlag{
			Name:    "head-lag",
			EnvVars: []string{"LOTUS_STATS_HEAD_LAG"},
			Usage:   "the number of tipsets to delay processing on to smooth chain reorgs",
			Value:   int(buildconstants.MessageConfidence),
		},
		&cli.BoolFlag{
			Name:    "no-sync",
			EnvVars: []string{"LOTUS_STATS_NO_SYNC"},
			Usage:   "do not wait for chain sync to complete",
			Value:   false,
		},
		&cli.IntFlag{
			Name:    "ipld-store-cache-size",
			Usage:   "size of lru cache for ChainReadObj",
			EnvVars: []string{"LOTUS_STATS_IPLD_STORE_CACHE_SIZE"},
			Value:   2 << 15,
		},
		&cli.StringFlag{
			Name:  "http-server-timeout",
			Value: "30s",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()

		resetFlag := cctx.Bool("reset")
		noSyncFlag := cctx.Bool("no-sync")
		heightFlag := cctx.Int("height")
		headLagFlag := cctx.Int("head-lag")

		influxHostnameFlag := cctx.String("influx-hostname")
		influxUsernameFlag := cctx.String("influx-username")
		influxPasswordFlag := cctx.String("influx-password")
		influxDatabaseFlag := cctx.String("influx-database")

		ipldStoreCacheSizeFlag := cctx.Int("ipld-store-cache-size")

		log.Infow("opening influx client", "hostname", influxHostnameFlag, "username", influxUsernameFlag, "database", influxDatabaseFlag)

		influxClient, err := influx.NewClient(influxHostnameFlag, influxUsernameFlag, influxPasswordFlag)
		if err != nil {
			return err
		}

		exporter, err := prometheus.NewExporter(prometheus.Options{
			Namespace: "lotus_stats",
		})
		if err != nil {
			return err
		}

		timeout, err := time.ParseDuration(cctx.String("http-server-timeout"))
		if err != nil {
			return xerrors.Errorf("invalid time string %s: %x", cctx.String("http-server-timeout"), err)
		}

		go func() {
			http.Handle("/metrics", exporter)
			server := &http.Server{
				Addr:              ":6688",
				ReadHeaderTimeout: timeout,
			}
			if err := server.ListenAndServe(); err != nil {
				log.Errorw("failed to start http server", "err", err)
			}
		}()

		if resetFlag {
			if err := influx.ResetDatabase(influxClient, influxDatabaseFlag); err != nil {
				return err
			}
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		if !noSyncFlag {
			if err := sync.SyncWait(ctx, api); err != nil {
				return err
			}
		}

		gtp, err := api.ChainGetGenesis(ctx)
		if err != nil {
			return err
		}

		genesisTime := time.Unix(int64(gtp.MinTimestamp()), 0)

		// When height is set to `0` we will resume from the best height we can.
		// The goal is to ensure we have data in the last 60 tipsets
		height := int64(heightFlag)
		if !resetFlag && height == 0 {
			lastHeight, err := influx.GetLastRecordedHeight(influxClient, influxDatabaseFlag)
			if err != nil {
				return err
			}

			sinceGenesis := build.Clock.Now().Sub(genesisTime)
			expectedHeight := int64(sinceGenesis.Seconds()) / int64(buildconstants.BlockDelaySecs)

			startOfWindowHeight := expectedHeight - 60

			if lastHeight > startOfWindowHeight {
				height = lastHeight
			} else {
				height = startOfWindowHeight
			}

			ts, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}

			headHeight := int64(ts.Height())
			if headHeight < height {
				height = headHeight
			}
		}

		go func() {
			t := time.NewTicker(time.Second)

			for {
				select {
				case <-t.C:
					sinceGenesis := build.Clock.Now().Sub(genesisTime)
					expectedHeight := int64(sinceGenesis.Seconds()) / int64(buildconstants.BlockDelaySecs)

					stats.Record(ctx, metrics.TipsetCollectionHeightExpected.M(expectedHeight))
				}
			}
		}()

		store, err := ipldstore.NewApiIpldStore(ctx, api, ipldStoreCacheSizeFlag)
		if err != nil {
			return err
		}

		collector, err := points.NewChainPointCollector(ctx, store, api)
		if err != nil {
			return err
		}

		tipsets, err := sync.BufferedTipsetChannel(ctx, api, abi.ChainEpoch(height), headLagFlag)
		if err != nil {
			return err
		}

		wq := influx.NewWriteQueue(ctx, influxClient)
		defer wq.Close()

		for tipset := range tipsets {
			if nb, err := collector.Collect(ctx, tipset); err != nil {
				log.Warnw("failed to collect points", "err", err)
			} else {
				nb.SetDatabase(influxDatabaseFlag)
				wq.AddBatch(nb)
			}
		}

		return nil
	},
}
