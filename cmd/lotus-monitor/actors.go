package main

import (
	"errors"

	"github.com/filecoin-project/go-address"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/types"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	keyAddress                       = tag.MustNewKey("actorAddress")
	actorBalanceMetric               = stats.Float64("actor/Balance", "actor blance", "FIL")
	actorNonceMetric                 = stats.Int64("actor/Nonce", "nonce", "n")
	actorVerifiedClientDataCapMetric = stats.Int64(
		"actor/VerifiedClientDataCap",
		"verified client datacap",
		stats.UnitBytes,
	)
	actorVerifierDataCapMetric = stats.Int64(
		"actor/VerifierDataCap",
		"verifier datacap",
		stats.UnitBytes,
	)
	actorBalanceView = &view.View{
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
	actorVerifiedClientDataCapView = &view.View{
		Name:        "actor/VerifiedClientDataCap",
		Measure:     actorVerifiedClientDataCapMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
	}
	actorVerifierDataCapView = &view.View{
		Name:        "actor/VerifierDataCap",
		Measure:     actorVerifierDataCapMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyAddress},
	}
)

func init() {
	if err := view.Register(
		actorBalanceView,
		actorNonceView,
		actorVerifiedClientDataCapView,
		actorVerifierDataCapView,
	); err != nil {
		log.Fatalf("cannot register actor views: %w", err)
	}
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
		errs <- errors.New("actor not found")
	} else {
		stats.Record(ctx, actorBalanceMetric.M(filBalance(actor.Balance)))
		stats.Record(ctx, actorNonceMetric.M(int64(actor.Nonce)))
	}

	verified, verifier, err := getDatacap(ctx, api, addr)
	if err != nil {
		log.Warnw("encountered an error looking up datacap", "addr", addr, "err", err)
		errs <- err
	} else {
		stats.Record(ctx, actorVerifiedClientDataCapMetric.M(verified.Int64()))
		stats.Record(ctx, actorVerifierDataCapMetric.M(verifier.Int64()))
	}
}
