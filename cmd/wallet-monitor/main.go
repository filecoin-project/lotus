package main

import (
	"math/big"
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
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

var (
	log = logging.Logger("main")

	keyAddress  = tag.MustNewKey("address")
	balMetric   = stats.Int64("actor/balance", "actor blance", "FIL")
	nonceMetric = stats.Int64("actor/nonce", "nonce", "n")
	balanceView = &view.View{
		Name:        "actor/balance",
		Measure:     balMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
	}
	nonceView = &view.View{
		Name:        "actor/nonce",
		Measure:     nonceMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
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

			if err := view.Register(balanceView, nonceView); err != nil {
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
					ctx, _ := tag.New(cctx.Context, tag.Insert(keyAddress, addr.String()))
					actor, err := api.StateGetActor(ctx, addr, types.TipSetKey{})
					if err != nil {
						log.Warnw("could not get actor", "address", arg, "err", err)
					}
					if actor == nil {
						log.Warnw("actor not found", "actor", arg)
						continue
					}
					var fil big.Int
					fil.Div(actor.Balance.Int, big.NewInt(1000000000000000000))
					stats.Record(ctx, balMetric.M(fil.Int64()))
					stats.Record(ctx, nonceMetric.M(int64(actor.Nonce)))
				}
			}

			return nil
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalf("%+v", err)
	}
}
