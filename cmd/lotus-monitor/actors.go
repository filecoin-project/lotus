package main

import (
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	keyAddress         = tag.MustNewKey("actorAddress")
	actorBalanceMetric = stats.Int64("actor/Balance", "actor blance", "FIL")
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
		"bytes",
	)
	actorQualityAdjPowerView = &view.View{
		Name:        "actor/QualityAdjPower",
		Measure:     actorRawPowerMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyMinerAddress},
	}
	actorRawPowerMetric = stats.Int64(
		"actor/RawPower",
		"raw power",
		"bytes",
	)
	actorRawPowerView = &view.View{
		Name:        "actor/RawPower",
		Measure:     actorRawPowerMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyMinerAddress},
	}
)

func init() {
	view.Register(actorBalanceView, actorNonceView, actorQualityAdjPowerView, actorRawPowerView)
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

			hasDataCap, power, err := dataCap(cctx, api, addr)
			if err != nil {
				log.Warnw("encountered an error looking up datacap", "addr", addr, "err", err)
				errs <- err
				continue
			}
			if hasDataCap {
				stats.Record(ctx, actorQualityAdjPowerMetric.M(power.Int64()))
				stats.Record(ctx, actorRawPowerMetric.M(power.Int64()))
			}
		}
	}
}

func dataCap(cctx *cli.Context, api v0api.FullNode, addr address.Address) (bool, abi.StoragePower, error) {
	ctx := cctx.Context
	tsk, err := lcli.LoadTipSet(ctx, cctx, api)
	id, err := api.StateLookupID(ctx, addr, tsk.Key())
	if err != nil {
		return false, big.Zero(), err
	}

	actor, err := api.StateGetActor(ctx, verifreg.Address, tsk.Key())
	if err != nil {
		return false, big.Zero(), err
	}

	apibs := blockstore.NewAPIBlockstore(api)
	store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

	s, err := verifreg.Load(store, actor)
	if err != nil {
		return false, big.Zero(), err
	}

	return s.VerifierDataCap(id)
}
