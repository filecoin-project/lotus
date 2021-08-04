package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	keyMinerAddress            = tag.MustNewKey("minerAddress")
	minerQualityAdjPowerMetric = stats.Int64(
		"miner/QualityAdjPower",
		"quality-adjusted power",
		stats.UnitBytes,
	)
	minerQualityAdjPowerView = &view.View{
		Name:        "miner/QualityAdjPower",
		Measure:     minerQualityAdjPowerMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyMinerAddress},
	}
	minerRawPowerMetric = stats.Int64(
		"miner/RawPower",
		"raw power",
		stats.UnitBytes,
	)
	minerRawPowerView = &view.View{
		Name:        "miner/RawPower",
		Measure:     minerRawPowerMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyMinerAddress},
	}
	minerFaultCountMetric = stats.Int64(
		"miner/Faults",
		"faults",
		"faults",
	)
	minerFaultCountView = &view.View{
		Name:        "miner/Faults",
		Measure:     minerFaultCountMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyMinerAddress},
	}
	minerRecoveryCountMetric = stats.Int64(
		"miner/Recoveries",
		"recoveries",
		"recoveries",
	)
	minerRecoveryCountView = &view.View{
		Name:        "miner/Recoveries",
		Measure:     minerRecoveryCountMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyMinerAddress},
	}
	minerBalanceMetric = stats.Float64(
		"miner/Balance",
		"miner fil balance",
		"FIL",
	)
	minerBalanceView = &view.View{
		Name:        "miner/Balance",
		Measure:     minerBalanceMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyMinerAddress},
	}
)

func init() {
	view.Register(minerQualityAdjPowerView, minerRawPowerView, minerFaultCountView, minerRecoveryCountView, minerBalanceView)
}

func minerRecorder(cctx *cli.Context, addr address.Address, api v0api.FullNode, errs chan error) {
	ctx, _ := tag.New(cctx.Context, tag.Upsert(keyMinerAddress, addr.String()))
	mp, err := api.StateMinerPower(ctx, addr, types.EmptyTSK)
	if err != nil {
		log.Warnw("could not get miner power", "address", addr, "err", err)
		errs <- err
	}
	stats.Record(ctx, minerQualityAdjPowerMetric.M(mp.MinerPower.QualityAdjPower.Int64()))
	stats.Record(ctx, minerRawPowerMetric.M(mp.MinerPower.RawBytePower.Int64()))

	if !cctx.Bool("gateway-api") {
		mf, err := api.StateMinerFaults(ctx, addr, types.EmptyTSK)
		if err != nil {
			log.Warnw("could not get miner faults", "address", addr, "err", err)
			errs <- err
		}
		faultCount, err := mf.Count()
		if err != nil {
			log.Warnw("could not get fault count", "address", addr, "err", err)
			errs <- err
		}
		stats.Record(ctx, minerFaultCountMetric.M(int64(faultCount)))

		mr, err := api.StateMinerRecoveries(ctx, addr, types.EmptyTSK)
		if err != nil {
			log.Warnw("could not get miner recoveries", "address", addr, "err", err)
			errs <- err
		}
		recoveryCount, err := mr.Count()
		if err != nil {
			log.Warnw("could not get revoery count", "address", addr, "err", err)
			errs <- err
		}

		stats.Record(ctx, minerRecoveryCountMetric.M(int64(recoveryCount)))
		ma, err := api.StateMinerAvailableBalance(ctx, addr, types.EmptyTSK)
		if err != nil {
			log.Warnw("could not get miner available balance", "address", addr, "err", err)
			errs <- err
		}
		stats.Record(ctx, minerBalanceMetric.M(filBalance(ma)))
	}
}
