package node

import (
	"errors"
	"time"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	"github.com/filecoin-project/go-state-types/abi"
	storage2 "github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/markets/dagstore"
	"github.com/filecoin-project/lotus/markets/dealfilter"
	"github.com/filecoin-project/lotus/markets/retrievaladapter"
	"github.com/filecoin-project/lotus/markets/sectoraccessor"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

var MinerNode = Options(
	Override(new(sectorstorage.StorageAuth), modules.StorageAuth),

	// Actor config
	Override(new(dtypes.MinerAddress), modules.MinerAddress),
	Override(new(dtypes.MinerID), modules.MinerID),
	Override(new(abi.RegisteredSealProof), modules.SealProofType),
	Override(new(dtypes.NetworkName), modules.StorageNetworkName),

	// Mining / proving
	Override(new(*storage.AddressSelector), modules.AddressSelector(nil)),
)

func ConfigStorageMiner(c interface{}) Option {
	cfg, ok := c.(*config.StorageMiner)
	if !ok {
		return Error(xerrors.Errorf("invalid config from repo, got: %T", c))
	}

	pricingConfig := cfg.Dealmaking.RetrievalPricing
	if pricingConfig.Strategy == config.RetrievalPricingExternalMode {
		if pricingConfig.External == nil {
			return Error(xerrors.New("retrieval pricing policy has been to set to external but external policy config is nil"))
		}

		if pricingConfig.External.Path == "" {
			return Error(xerrors.New("retrieval pricing policy has been to set to external but external script path is empty"))
		}
	} else if pricingConfig.Strategy != config.RetrievalPricingDefaultMode {
		return Error(xerrors.New("retrieval pricing policy must be either default or external"))
	}

	enableLibp2pNode := cfg.Subsystems.EnableMarkets // we enable libp2p nodes if the storage market subsystem is enabled, otherwise we don't

	return Options(
		ConfigCommon(&cfg.Common, enableLibp2pNode),

		Override(CheckFDLimit, modules.CheckFdLimit(build.MinerFDLimit)), // recommend at least 100k FD limit to miners

		Override(new(api.MinerSubsystems), modules.ExtractEnabledMinerSubsystems(cfg.Subsystems)),
		Override(new(stores.LocalStorage), From(new(repo.LockedRepo))),
		Override(new(*stores.Local), modules.LocalStorage),
		Override(new(*stores.Remote), modules.RemoteStorage),
		Override(new(dtypes.RetrievalPricingFunc), modules.RetrievalPricingFunc(cfg.Dealmaking)),

		If(!cfg.Subsystems.EnableMining,
			If(cfg.Subsystems.EnableSealing, Error(xerrors.Errorf("sealing can only be enabled on a mining node"))),
			If(cfg.Subsystems.EnableSectorStorage, Error(xerrors.Errorf("sealing can only be enabled on a mining node"))),
		),
		If(cfg.Subsystems.EnableMining,
			If(!cfg.Subsystems.EnableSealing, Error(xerrors.Errorf("sealing can't be disabled on a mining node yet"))),
			If(!cfg.Subsystems.EnableSectorStorage, Error(xerrors.Errorf("sealing can't be disabled on a mining node yet"))),

			// Sector storage: Proofs
			Override(new(ffiwrapper.Verifier), ffiwrapper.ProofVerifier),
			Override(new(ffiwrapper.Prover), ffiwrapper.ProofProver),
			Override(new(storage2.Prover), From(new(sectorstorage.SectorManager))),

			// Sealing (todo should be under EnableSealing, but storagefsm is currently bundled with storage.Miner)
			Override(new(sealing.SectorIDCounter), modules.SectorIDCounter),
			Override(GetParamsKey, modules.GetParams),

			Override(new(dtypes.SetSealingConfigFunc), modules.NewSetSealConfigFunc),
			Override(new(dtypes.GetSealingConfigFunc), modules.NewGetSealConfigFunc),

			// Mining / proving
			Override(new(*slashfilter.SlashFilter), modules.NewSlashFilter),
			Override(new(*storage.Miner), modules.StorageMiner(config.DefaultStorageMiner().Fees)),
			Override(new(*miner.Miner), modules.SetupBlockProducer),
			Override(new(gen.WinningPoStProver), storage.NewWinningPoStProver),
			Override(new(*storage.Miner), modules.StorageMiner(cfg.Fees)),
			Override(new(sectorblocks.SectorBuilder), From(new(*storage.Miner))),
		),

		If(cfg.Subsystems.EnableSectorStorage,
			// Sector storage
			Override(new(*stores.Index), stores.NewIndex),
			Override(new(stores.SectorIndex), From(new(*stores.Index))),
			Override(new(*sectorstorage.Manager), modules.SectorStorage),
			Override(new(sectorstorage.Unsealer), From(new(*sectorstorage.Manager))),
			Override(new(sectorstorage.SectorManager), From(new(*sectorstorage.Manager))),
			Override(new(storiface.WorkerReturn), From(new(sectorstorage.SectorManager))),
		),

		If(!cfg.Subsystems.EnableSectorStorage,
			Override(new(sectorstorage.StorageAuth), modules.StorageAuthWithURL(cfg.Subsystems.SectorIndexApiInfo)),
			Override(new(modules.MinerStorageService), modules.ConnectStorageService(cfg.Subsystems.SectorIndexApiInfo)),
			Override(new(sectorstorage.Unsealer), From(new(modules.MinerStorageService))),
			Override(new(sectorblocks.SectorBuilder), From(new(modules.MinerStorageService))),
		),
		If(!cfg.Subsystems.EnableSealing,
			Override(new(modules.MinerSealingService), modules.ConnectSealingService(cfg.Subsystems.SealerApiInfo)),
			Override(new(stores.SectorIndex), From(new(modules.MinerSealingService))),
		),

		If(cfg.Subsystems.EnableMarkets,
			// Markets
			Override(new(dtypes.StagingBlockstore), modules.StagingBlockstore),
			Override(new(dtypes.StagingGraphsync), modules.StagingGraphsync(cfg.Dealmaking.SimultaneousTransfersForStorage, cfg.Dealmaking.SimultaneousTransfersForRetrieval)),
			Override(new(dtypes.ProviderPieceStore), modules.NewProviderPieceStore),
			Override(new(*sectorblocks.SectorBlocks), sectorblocks.NewSectorBlocks),

			// Markets (retrieval deps)
			Override(new(sectorstorage.PieceProvider), sectorstorage.NewPieceProvider),
			Override(new(dtypes.RetrievalPricingFunc), modules.RetrievalPricingFunc(config.DealmakingConfig{
				RetrievalPricing: &config.RetrievalPricing{
					Strategy: config.RetrievalPricingDefaultMode,
					Default:  &config.RetrievalPricingDefault{},
				},
			})),
			Override(new(dtypes.RetrievalPricingFunc), modules.RetrievalPricingFunc(cfg.Dealmaking)),

			// DAG Store
			Override(new(dagstore.MinerAPI), modules.NewMinerAPI),
			Override(DAGStoreKey, modules.DAGStore),

			// Markets (retrieval)
			Override(new(dagstore.SectorAccessor), sectoraccessor.NewSectorAccessor),
			Override(new(retrievalmarket.SectorAccessor), From(new(dagstore.SectorAccessor))),
			Override(new(retrievalmarket.RetrievalProviderNode), retrievaladapter.NewRetrievalProviderNode),
			Override(new(rmnet.RetrievalMarketNetwork), modules.RetrievalNetwork),
			Override(new(retrievalmarket.RetrievalProvider), modules.RetrievalProvider),
			Override(new(dtypes.RetrievalDealFilter), modules.RetrievalDealFilter(nil)),
			Override(HandleRetrievalKey, modules.HandleRetrieval),

			// Markets (storage)
			Override(new(dtypes.ProviderDataTransfer), modules.NewProviderDAGServiceDataTransfer),
			Override(new(*storedask.StoredAsk), modules.NewStorageAsk),
			Override(new(dtypes.StorageDealFilter), modules.BasicDealFilter(cfg.Dealmaking, nil)),
			Override(new(storagemarket.StorageProvider), modules.StorageProvider),
			Override(new(*storageadapter.DealPublisher), storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{})),
			Override(HandleMigrateProviderFundsKey, modules.HandleMigrateProviderFunds),
			Override(HandleDealsKey, modules.HandleDeals),

			// Config (todo: get a real property system)
			Override(new(dtypes.ConsiderOnlineStorageDealsConfigFunc), modules.NewConsiderOnlineStorageDealsConfigFunc),
			Override(new(dtypes.SetConsiderOnlineStorageDealsConfigFunc), modules.NewSetConsideringOnlineStorageDealsFunc),
			Override(new(dtypes.ConsiderOnlineRetrievalDealsConfigFunc), modules.NewConsiderOnlineRetrievalDealsConfigFunc),
			Override(new(dtypes.SetConsiderOnlineRetrievalDealsConfigFunc), modules.NewSetConsiderOnlineRetrievalDealsConfigFunc),
			Override(new(dtypes.StorageDealPieceCidBlocklistConfigFunc), modules.NewStorageDealPieceCidBlocklistConfigFunc),
			Override(new(dtypes.SetStorageDealPieceCidBlocklistConfigFunc), modules.NewSetStorageDealPieceCidBlocklistConfigFunc),
			Override(new(dtypes.ConsiderOfflineStorageDealsConfigFunc), modules.NewConsiderOfflineStorageDealsConfigFunc),
			Override(new(dtypes.SetConsiderOfflineStorageDealsConfigFunc), modules.NewSetConsideringOfflineStorageDealsFunc),
			Override(new(dtypes.ConsiderOfflineRetrievalDealsConfigFunc), modules.NewConsiderOfflineRetrievalDealsConfigFunc),
			Override(new(dtypes.SetConsiderOfflineRetrievalDealsConfigFunc), modules.NewSetConsiderOfflineRetrievalDealsConfigFunc),
			Override(new(dtypes.ConsiderVerifiedStorageDealsConfigFunc), modules.NewConsiderVerifiedStorageDealsConfigFunc),
			Override(new(dtypes.SetConsiderVerifiedStorageDealsConfigFunc), modules.NewSetConsideringVerifiedStorageDealsFunc),
			Override(new(dtypes.ConsiderUnverifiedStorageDealsConfigFunc), modules.NewConsiderUnverifiedStorageDealsConfigFunc),
			Override(new(dtypes.SetConsiderUnverifiedStorageDealsConfigFunc), modules.NewSetConsideringUnverifiedStorageDealsFunc),
			Override(new(dtypes.SetExpectedSealDurationFunc), modules.NewSetExpectedSealDurationFunc),
			Override(new(dtypes.GetExpectedSealDurationFunc), modules.NewGetExpectedSealDurationFunc),
			Override(new(dtypes.SetMaxDealStartDelayFunc), modules.NewSetMaxDealStartDelayFunc),
			Override(new(dtypes.GetMaxDealStartDelayFunc), modules.NewGetMaxDealStartDelayFunc),

			If(cfg.Dealmaking.Filter != "",
				Override(new(dtypes.StorageDealFilter), modules.BasicDealFilter(cfg.Dealmaking, dealfilter.CliStorageDealFilter(cfg.Dealmaking.Filter))),
			),

			If(cfg.Dealmaking.RetrievalFilter != "",
				Override(new(dtypes.RetrievalDealFilter), modules.RetrievalDealFilter(dealfilter.CliRetrievalDealFilter(cfg.Dealmaking.RetrievalFilter))),
			),
			Override(new(*storageadapter.DealPublisher), storageadapter.NewDealPublisher(&cfg.Fees, storageadapter.PublishMsgConfig{
				Period:                  time.Duration(cfg.Dealmaking.PublishMsgPeriod),
				MaxDealsPerMsg:          cfg.Dealmaking.MaxDealsPerPublishMsg,
				StartEpochSealingBuffer: cfg.Dealmaking.StartEpochSealingBuffer,
			})),
			Override(new(storagemarket.StorageProviderNode), storageadapter.NewProviderNodeAdapter(&cfg.Fees, &cfg.Dealmaking)),
		),

		Override(new(sectorstorage.SealerConfig), cfg.Storage),
		Override(new(*storage.AddressSelector), modules.AddressSelector(&cfg.Addresses)),
	)
}

func StorageMiner(out *api.StorageMiner, subsystemsCfg config.MinerSubsystemConfig) Option {
	return Options(
		ApplyIf(func(s *Settings) bool { return s.Config },
			Error(errors.New("the StorageMiner option must be set before Config option")),
		),

		func(s *Settings) error {
			s.nodeType = repo.StorageMiner
			s.enableLibp2pNode = subsystemsCfg.EnableMarkets
			return nil
		},

		func(s *Settings) error {
			resAPI := &impl.StorageMinerAPI{}
			s.invokes[ExtractApiKey] = fx.Populate(resAPI)
			*out = resAPI
			return nil
		},
	)
}
