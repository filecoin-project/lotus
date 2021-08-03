package main

import (
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api/v0api"
	lcli "github.com/filecoin-project/lotus/cli"
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
		"bytes",
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
		"bytes",
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
		"faults",
		"faults",
	)
	minerRecoveryCountView = &view.View{
		Name:        "miner/Recoveries",
		Measure:     minerRecoveryCountMetric,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{keyMinerAddress},
	}
	minerBalanceMetric = stats.Int64(
		"miner/Recoveries",
		"faults",
		"faults",
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

func minerRecorder(cctx *cli.Context, api v0api.FullNode, errs chan error) {
	minerAddrArgs := cctx.StringSlice("miners")
	minerAddrs := make([]address.Address, len(minerAddrArgs))
	for i, a := range minerAddrArgs {
		addr, err := address.NewFromString(a)
		if err != nil {
			log.Warnw("invalid address will not be monitored", "address", a, "err", err)
			errs <- err
		}
		minerAddrs[i] = addr
	}
	for range time.Tick(cctx.Duration("poll")) {
		for _, addr := range minerAddrs {
			ctx, _ := tag.New(cctx.Context, tag.Insert(keyMinerAddress, addr.String()))
			tsk, err := lcli.LoadTipSet(ctx, cctx, api)
			if err != nil {
				log.Warnw("could not load tipset key, skipping this poll")
				errs <- err
			}

			mp, err := api.StateMinerPower(ctx, addr, tsk.Key())
			if err != nil {
				log.Warnw("could not get miner power", "address", addr, "err", err)
				errs <- err
			}
			stats.Record(ctx, minerQualityAdjPowerMetric.M(mp.MinerPower.QualityAdjPower.Int64()))
			stats.Record(ctx, minerRawPowerMetric.M(mp.MinerPower.RawBytePower.Int64()))

			if !cctx.Bool("gateway-api") {
				mf, err := api.StateMinerFaults(ctx, addr, tsk.Key())
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

				mr, err := api.StateMinerRecoveries(ctx, addr, tsk.Key())
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
				ma, err := api.StateMinerAvailableBalance(ctx, addr, tsk.Key())
				if err != nil {
					log.Warnw("could not get miner available balance", "address", addr, "err", err)
					errs <- err
				}
				stats.Record(ctx, minerBalanceMetric.M(ma.Int64()))
			}
		}
	}
}
