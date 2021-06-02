package main

import (
	"net/http"
	"os"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

var (
	log = logging.Logger("main")

	keyWallet   = tag.MustNewKey("wallet")
	balMetric   = stats.Int64("wallet/balance", "wallet blance", "FIL")
	balanceView = &view.View{
		Name:        "wallet/balance",
		Measure:     balMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyWallet},
	}
)

func main() {
	app := &cli.App{
		Name:        "wallet-monitor",
		Usage:       "monitor wallet addresses",
		Version:     build.UserVersion(),
		Description: "Export wallet balances as metrics",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "listen",
				Value: "0.0.0.0:8888",
			},
		},
		Action: func(cctx *cli.Context) error {
			api, closer, err := cliutil.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			if err := view.Register(balanceView); err != nil {
				return err
			}

			pe, err := prometheus.NewExporter(prometheus.Options{
				Namespace: "walletmonitor",
			})
			if err != nil {
				log.Fatalw("failed to create the Prometheus stats exporter", "err", err)
			}

			go func() {
				mux := http.NewServeMux()
				mux.Handle("/metrics", pe)
				if err := http.ListenAndServe(cctx.String("listen"), mux); err != nil {
					log.Fatalw("failed to run endpoint", "err", err)
				}
			}()

			for range time.Tick(time.Minute) {
				for _, arg := range cctx.Args().Slice() {
					addr, err := address.NewFromString(arg)
					if err != nil {
						log.Warnw("invalid address will not be monitored", "address", arg, "err", err)
					}
					ctx, _ := tag.New(cctx.Context, tag.Insert(keyWallet, addr.String()))
					bal, err := api.WalletBalance(ctx, addr)
					if err != nil {
						log.Warnf("could not get balance", "address", arg, "err", err)
					}
					stats.Record(ctx, balMetric.M(bal.Int64()))
				}
			}

			return nil
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalf("%+v", err)
	}
}
