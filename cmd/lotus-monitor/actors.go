package main

import (
	"math/big"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	keyAddress         = tag.MustNewKey("actorAddress")
	actorBalanceMetric = stats.Int64("actor/Balance", "actor blance", "FIL")
	actorNonceMetric   = stats.Int64("actor/Nonce", "nonce", "n")
	balanceView        = &view.View{
		Name:        "actor/Balance",
		Measure:     actorBalanceMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
	}
	nonceView = &view.View{
		Name:        "actor/Nonce",
		Measure:     actorNonceMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
	}
)

func init() {
	view.Register(balanceView, nonceView)
}

func actorRecorder(cctx *cli.Context, api v0api.FullNode, errs chan error) {
	actorAddrArgs := cctx.StringSlice("actors")
	actorAddrs := make([]address.Address, len(actorAddrArgs))
	for i, a := range actorAddrArgs {
		addr, err := address.NewFromString(a)
		if err != nil {
			log.Warnw("invalid address will not be monitored", "address", a, "err", err)
			errs <- err
		}
		actorAddrs[i] = addr
	}
	for range time.Tick(cctx.Duration("poll")) {
		for _, addr := range actorAddrs {
			ctx, _ := tag.New(cctx.Context, tag.Insert(keyAddress, addr.String()))
			actor, err := api.StateGetActor(ctx, addr, types.TipSetKey{})
			if err != nil {
				log.Warnw("could not get actor", "address", addr, "err", err)
				errs <- err
			}
			if actor == nil {
				log.Warnw("actor not found", "actor", addr)
				errs <- err
				continue
			}
			var fil big.Int
			p := big.NewInt(int64(build.FilecoinPrecision))
			fil.Div(actor.Balance.Int, p)
			stats.Record(ctx, actorBalanceMetric.M(fil.Int64()))
			stats.Record(ctx, actorNonceMetric.M(int64(actor.Nonce)))
		}
	}
}
