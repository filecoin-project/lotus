package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/types"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	keyAddress         = tag.MustNewKey("actorAddress")
	actorBalanceMetric = stats.Float64("actor/Balance", "actor blance", "FIL")
	actorNonceMetric   = stats.Int64("actor/Nonce", "nonce", "n")
	actorBalanceView   = &view.View{
		Name:        "actor/Balance",
		Measure:     actorBalanceMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
	}
	actorNonceView = &view.View{
		Name:        "actor/Nonce",
		Measure:     actorNonceMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
	}
	actorQualityAdjPowerMetric = stats.Int64(
		"actor/QualityAdjPower",
		"quality-adjusted power",
		stats.UnitBytes,
	)
	actorQualityAdjPowerView = &view.View{
		Name:        "actor/QualityAdjPower",
		Measure:     actorRawPowerMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
	}
	actorRawPowerMetric = stats.Int64(
		"actor/RawPower",
		"raw power",
		stats.UnitBytes,
	)
	actorRawPowerView = &view.View{
		Name:        "actor/RawPower",
		Measure:     actorRawPowerMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
	}
	actorDatacapMetric = stats.Int64(
		"actor/Datacap",
		"client datacap",
		stats.UnitBytes,
	)
	actorDatacapView = &view.View{
		Name:        "actor/Datacap",
		Measure:     actorDatacapMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
	}
)

func init() {
	view.Register(actorBalanceView, actorNonceView, actorQualityAdjPowerView, actorRawPowerView, actorDatacapView)
}

func actorRecorder(cctx *cli.Context, addr address.Address, api v0api.FullNode, errs chan error) {
	ctx, _ := tag.New(cctx.Context, tag.Upsert(keyAddress, addr.String()))
	actor, err := api.StateGetActor(ctx, addr, types.EmptyTSK)
	if err != nil {
		log.Warnw("could not get actor", "address", addr, "err", err)
		errs <- err
	}
	if actor == nil {
		log.Warnw("actor not found", "actor", addr)
		errs <- err
	}
	stats.Record(ctx, actorBalanceMetric.M(filBalance(actor.Balance)))
	stats.Record(ctx, actorNonceMetric.M(int64(actor.Nonce)))

	_, power, err := verifiedPower(ctx, api, addr)
	if err != nil {
		log.Warnw("encountered an error looking up power", "addr", addr, "err", err)
		errs <- err
	}
	stats.Record(ctx, actorQualityAdjPowerMetric.M(power.Int64()))
	stats.Record(ctx, actorRawPowerMetric.M(power.Int64()))

	dcap, err := api.StateVerifiedClientStatus(ctx, addr, types.EmptyTSK)
	if err != nil {
		log.Warnw("encountered an error looking up datacap", "addr", addr, "err", err)
		errs <- err
	}
	var dcap64 int64
	if dcap != nil {
		dcap64 = dcap.Int64()
	}
	stats.Record(ctx, actorDatacapMetric.M(dcap64))
}
