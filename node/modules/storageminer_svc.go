package modules

import (
	"context"
	"strings"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/provider/lpmarket"
	"github.com/filecoin-project/lotus/provider/lpmarket/fakelm"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

type MinerSealingService api.StorageMiner
type MinerStorageService api.StorageMiner

var _ sectorblocks.SectorBuilder = *new(MinerSealingService)

func harmonyApiInfoToConf(apiInfo string) (config.HarmonyDB, error) {
	hc := config.HarmonyDB{}

	// apiInfo - harmony:layer:maddr:user:pass:dbname:host:port

	parts := strings.Split(apiInfo, ":")

	if len(parts) != 8 {
		return config.HarmonyDB{}, xerrors.Errorf("invalid harmonydb info '%s'", apiInfo)
	}

	hc.Username = parts[3]
	hc.Password = parts[4]
	hc.Database = parts[5]
	hc.Hosts = []string{parts[6]}
	hc.Port = parts[7]

	return hc, nil
}

func connectHarmony(apiInfo string, fapi v1api.FullNode, mctx helpers.MetricsCtx, lc fx.Lifecycle) (api.StorageMiner, error) {
	log.Info("Connecting to harmonydb")

	hc, err := harmonyApiInfoToConf(apiInfo)
	if err != nil {
		return nil, err
	}

	db, err := harmonydb.NewFromConfig(hc)
	if err != nil {
		return nil, xerrors.Errorf("connecting to harmonydb: %w", err)
	}

	parts := strings.Split(apiInfo, ":")
	maddr, err := address.NewFromString(parts[2])
	if err != nil {
		return nil, xerrors.Errorf("parsing miner address: %w", err)
	}

	pin := lpmarket.NewPieceIngester(db, fapi)

	si := paths.NewDBIndex(nil, db)

	mid, err := address.IDFromAddress(maddr)
	if err != nil {
		return nil, xerrors.Errorf("getting miner id: %w", err)
	}

	mi, err := fapi.StateMinerInfo(mctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("getting miner info: %w", err)
	}

	lp := fakelm.NewLMRPCProvider(si, fapi, maddr, abi.ActorID(mid), mi.SectorSize, pin, db, parts[1])

	ast := api.StorageMinerStruct{}

	ast.CommonStruct.Internal.AuthNew = lp.AuthNew

	ast.Internal.ActorAddress = lp.ActorAddress
	ast.Internal.WorkerJobs = lp.WorkerJobs
	ast.Internal.SectorsStatus = lp.SectorsStatus
	ast.Internal.SectorsList = lp.SectorsList
	ast.Internal.SectorsSummary = lp.SectorsSummary
	ast.Internal.SectorsListInStates = lp.SectorsListInStates
	ast.Internal.StorageRedeclareLocal = lp.StorageRedeclareLocal
	ast.Internal.ComputeDataCid = lp.ComputeDataCid
	ast.Internal.SectorAddPieceToAny = func(p0 context.Context, p1 abi.UnpaddedPieceSize, p2 storiface.Data, p3 api.PieceDealInfo) (api.SectorOffset, error) {
		panic("implement me")
	}

	ast.Internal.StorageList = si.StorageList
	ast.Internal.StorageDetach = si.StorageDetach
	ast.Internal.StorageReportHealth = si.StorageReportHealth
	ast.Internal.StorageDeclareSector = si.StorageDeclareSector
	ast.Internal.StorageDropSector = si.StorageDropSector
	ast.Internal.StorageFindSector = si.StorageFindSector
	ast.Internal.StorageInfo = si.StorageInfo
	ast.Internal.StorageBestAlloc = si.StorageBestAlloc
	ast.Internal.StorageLock = si.StorageLock
	ast.Internal.StorageTryLock = si.StorageTryLock
	ast.Internal.StorageGetLocks = si.StorageGetLocks

	return &ast, nil
}

func connectMinerService(apiInfo string) func(mctx helpers.MetricsCtx, lc fx.Lifecycle, fapi v1api.FullNode) (api.StorageMiner, error) {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, fapi v1api.FullNode) (api.StorageMiner, error) {
		if strings.HasPrefix(apiInfo, "harmony:") {
			return connectHarmony(apiInfo, fapi, mctx, lc)
		}

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
