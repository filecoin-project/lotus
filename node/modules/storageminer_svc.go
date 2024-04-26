package modules

import (
	"context"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/v1api"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

type MinerSealingService api.StorageMiner
type MinerStorageService api.StorageMiner

var _ sectorblocks.SectorBuilder = *new(MinerSealingService)

func connectMinerService(apiInfo string) func(mctx helpers.MetricsCtx, lc fx.Lifecycle, fapi v1api.FullNode) (api.StorageMiner, error) {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, fapi v1api.FullNode) (api.StorageMiner, error) {
		ctx := helpers.LifecycleCtx(mctx, lc)
		info := cliutil.ParseApiInfo(apiInfo)
		addr, err := info.DialArgs("v0")
		if err != nil {
			return nil, xerrors.Errorf("could not get DialArgs: %w", err)
		}

		log.Infof("Checking (svc) api version of %s", addr)

		mapi, closer, err := client.NewStorageMinerRPCV0(ctx, addr, info.AuthHeader())
		if err != nil {
			return nil, err
		}
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				v, err := mapi.Version(ctx)
				if err != nil {
					return xerrors.Errorf("checking version: %w", err)
				}

				if !v.APIVersion.EqMajorMinor(api.MinerAPIVersion0) {
					return xerrors.Errorf("remote service API version didn't match (expected %s, remote %s)", api.MinerAPIVersion0, v.APIVersion)
				}

				return nil
			},
			OnStop: func(context.Context) error {
				closer()
				return nil
			}})

		return mapi, nil
	}
}

func ConnectSealingService(apiInfo string) func(mctx helpers.MetricsCtx, lc fx.Lifecycle, fapi v1api.FullNode) (MinerSealingService, error) {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, fapi v1api.FullNode) (MinerSealingService, error) {
		log.Info("Connecting sealing service to miner")
		return connectMinerService(apiInfo)(mctx, lc, fapi)
	}
}

func ConnectStorageService(apiInfo string) func(mctx helpers.MetricsCtx, lc fx.Lifecycle, fapi v1api.FullNode) (MinerStorageService, error) {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, fapi v1api.FullNode) (MinerStorageService, error) {
		log.Info("Connecting storage service to miner")
		return connectMinerService(apiInfo)(mctx, lc, fapi)
	}
}
