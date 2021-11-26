package modules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/fx"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtnet "github.com/filecoin-project/go-data-transfer/network"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	piecefilestore "github.com/filecoin-project/go-fil-markets/filestore"
	piecestoreimpl "github.com/filecoin-project/go-fil-markets/piecestore/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/go-storedcounter"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	graphsync "github.com/ipfs/go-graphsync/impl"
	graphsyncimpl "github.com/ipfs/go-graphsync/impl"
	gsnet "github.com/ipfs/go-graphsync/network"
	"github.com/ipfs/go-graphsync/storeutil"
	"github.com/libp2p/go-libp2p-core/host"

	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/markets"
	"github.com/filecoin-project/lotus/markets/dagstore"
	marketevents "github.com/filecoin-project/lotus/markets/loggers"
	"github.com/filecoin-project/lotus/markets/pricing"
	lotusminer "github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage"
)

var (
	StorageCounterDSPrefix = "/storage/nextid"
	StagingAreaDirName     = "deal-staging"
)

func minerAddrFromDS(ds dtypes.MetadataDS) (address.Address, error) {
	maddrb, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return address.Undef, err
	}

	return address.NewFromBytes(maddrb)
}

func GetParams(spt abi.RegisteredSealProof) error {
	ssize, err := spt.SectorSize()
	if err != nil {
		return err
	}

	// If built-in assets are disabled, we expect the user to have placed the right
	// parameters in the right location on the filesystem (/var/tmp/filecoin-proof-parameters).
	if build.DisableBuiltinAssets {
		return nil
	}

	// TODO: We should fetch the params for the actual proof type, not just based on the size.
	if err := paramfetch.GetParams(context.TODO(), build.ParametersJSON(), build.SrsJSON(), uint64(ssize)); err != nil {
		return xerrors.Errorf("fetching proof parameters: %w", err)
	}

	return nil
}

func MinerAddress(ds dtypes.MetadataDS) (dtypes.MinerAddress, error) {
	ma, err := minerAddrFromDS(ds)
	return dtypes.MinerAddress(ma), err
}

func MinerID(ma dtypes.MinerAddress) (dtypes.MinerID, error) {
	id, err := address.IDFromAddress(address.Address(ma))
	return dtypes.MinerID(id), err
}

func StorageNetworkName(ctx helpers.MetricsCtx, a v1api.FullNode) (dtypes.NetworkName, error) {
	if !build.Devnet {
		return "testnetnet", nil
	}
	return a.StateNetworkName(ctx)
}

func SealProofType(maddr dtypes.MinerAddress, fnapi v1api.FullNode) (abi.RegisteredSealProof, error) {
	mi, err := fnapi.StateMinerInfo(context.TODO(), address.Address(maddr), types.EmptyTSK)
	if err != nil {
		return 0, err
	}
	networkVersion, err := fnapi.StateNetworkVersion(context.TODO(), types.EmptyTSK)
	if err != nil {
		return 0, err
	}

	return miner.PreferredSealProofTypeFromWindowPoStType(networkVersion, mi.WindowPoStProofType)
}

type sidsc struct {
	sc *storedcounter.StoredCounter
}

func (s *sidsc) Next() (abi.SectorNumber, error) {
	i, err := s.sc.Next()
	return abi.SectorNumber(i), err
}

func SectorIDCounter(ds dtypes.MetadataDS) sealing.SectorIDCounter {
	sc := storedcounter.New(ds, datastore.NewKey(StorageCounterDSPrefix))
	return &sidsc{sc}
}

func AddressSelector(addrConf *config.MinerAddressConfig) func() (*storage.AddressSelector, error) {
	return func() (*storage.AddressSelector, error) {
		as := &storage.AddressSelector{}
		if addrConf == nil {
			return as, nil
		}

		as.DisableOwnerFallback = addrConf.DisableOwnerFallback
		as.DisableWorkerFallback = addrConf.DisableWorkerFallback

		for _, s := range addrConf.PreCommitControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing precommit control address: %w", err)
			}

			as.PreCommitControl = append(as.PreCommitControl, addr)
		}

		for _, s := range addrConf.CommitControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing commit control address: %w", err)
			}

			as.CommitControl = append(as.CommitControl, addr)
		}

		for _, s := range addrConf.TerminateControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing terminate control address: %w", err)
			}

			as.TerminateControl = append(as.TerminateControl, addr)
		}

		for _, s := range addrConf.DealPublishControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing deal publishing control address: %w", err)
			}

			as.DealPublishControl = append(as.DealPublishControl, addr)
		}

		return as, nil
	}
}

type StorageMinerParams struct {
	fx.In

	Lifecycle          fx.Lifecycle
	MetricsCtx         helpers.MetricsCtx
	API                v1api.FullNode
	MetadataDS         dtypes.MetadataDS
	Sealer             sectorstorage.SectorManager
	SectorIDCounter    sealing.SectorIDCounter
	Verifier           ffiwrapper.Verifier
	Prover             ffiwrapper.Prover
	GetSealingConfigFn dtypes.GetSealingConfigFunc
	Journal            journal.Journal
	AddrSel            *storage.AddressSelector
}

func StorageMiner(fc config.MinerFeeConfig) func(params StorageMinerParams) (*storage.Miner, error) {
	return func(params StorageMinerParams) (*storage.Miner, error) {
		var (
			ds     = params.MetadataDS
			mctx   = params.MetricsCtx
			lc     = params.Lifecycle
			api    = params.API
			sealer = params.Sealer
			sc     = params.SectorIDCounter
			verif  = params.Verifier
			prover = params.Prover
			gsd    = params.GetSealingConfigFn
			j      = params.Journal
			as     = params.AddrSel
		)

		maddr, err := minerAddrFromDS(ds)
		if err != nil {
			return nil, err
		}

		ctx := helpers.LifecycleCtx(mctx, lc)

		fps, err := storage.NewWindowedPoStScheduler(api, fc, as, sealer, verif, sealer, j, maddr)
		if err != nil {
			return nil, err
		}

		sm, err := storage.NewMiner(api, maddr, ds, sealer, sc, verif, prover, gsd, fc, j, as)
		if err != nil {
			return nil, err
		}

		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				go fps.Run(ctx)
				return sm.Run(ctx)
			},
			OnStop: sm.Stop,
		})

		return sm, nil
	}
}

func HandleRetrieval(host host.Host, lc fx.Lifecycle, m retrievalmarket.RetrievalProvider, j journal.Journal) {
	m.OnReady(marketevents.ReadyLogger("retrieval provider"))
	lc.Append(fx.Hook{

		OnStart: func(ctx context.Context) error {
			m.SubscribeToEvents(marketevents.RetrievalProviderLogger)

			evtType := j.RegisterEventType("markets/retrieval/provider", "state_change")
			m.SubscribeToEvents(markets.RetrievalProviderJournaler(j, evtType))

			return m.Start(ctx)
		},
		OnStop: func(context.Context) error {
			return m.Stop()
		},
	})
}

func HandleDeals(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, h storagemarket.StorageProvider, j journal.Journal) {
	ctx := helpers.LifecycleCtx(mctx, lc)
	h.OnReady(marketevents.ReadyLogger("storage provider"))
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			h.SubscribeToEvents(marketevents.StorageProviderLogger)

			evtType := j.RegisterEventType("markets/storage/provider", "state_change")
			h.SubscribeToEvents(markets.StorageProviderJournaler(j, evtType))

			return h.Start(ctx)
		},
		OnStop: func(context.Context) error {
			return h.Stop()
		},
	})
}

func HandleMigrateProviderFunds(lc fx.Lifecycle, ds dtypes.MetadataDS, node api.FullNode, minerAddress dtypes.MinerAddress) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			b, err := ds.Get(datastore.NewKey("/marketfunds/provider"))
			if err != nil {
				if xerrors.Is(err, datastore.ErrNotFound) {
					return nil
				}
				return err
			}

			var value abi.TokenAmount
			if err = value.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
				return err
			}
			ts, err := node.ChainHead(ctx)
			if err != nil {
				log.Errorf("provider funds migration - getting chain head: %v", err)
				return nil
			}

			mi, err := node.StateMinerInfo(ctx, address.Address(minerAddress), ts.Key())
			if err != nil {
				log.Errorf("provider funds migration - getting miner info %s: %v", minerAddress, err)
				return nil
			}

			_, err = node.MarketReserveFunds(ctx, mi.Worker, address.Address(minerAddress), value)
			if err != nil {
				log.Errorf("provider funds migration - reserving funds (wallet %s, addr %s, funds %d): %v",
					mi.Worker, minerAddress, value, err)
				return nil
			}

			return ds.Delete(datastore.NewKey("/marketfunds/provider"))
		},
	})
}

// NewProviderDAGServiceDataTransfer returns a data transfer manager that just
// uses the provider's Staging DAG service for transfers
func NewProviderDAGServiceDataTransfer(lc fx.Lifecycle, h host.Host, gs dtypes.StagingGraphsync, ds dtypes.MetadataDS, r repo.LockedRepo) (dtypes.ProviderDataTransfer, error) {
	net := dtnet.NewFromLibp2pHost(h)

	dtDs := namespace.Wrap(ds, datastore.NewKey("/datatransfer/provider/transfers"))
	transport := dtgstransport.NewTransport(h.ID(), gs, net)
	err := os.MkdirAll(filepath.Join(r.Path(), "data-transfer"), 0755) //nolint: gosec
	if err != nil && !os.IsExist(err) {
		return nil, err
	}

	dt, err := dtimpl.NewDataTransfer(dtDs, filepath.Join(r.Path(), "data-transfer"), net, transport)
	if err != nil {
		return nil, err
	}

	dt.OnReady(marketevents.ReadyLogger("provider data transfer"))
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			dt.SubscribeToEvents(marketevents.DataTransferLogger)
			return dt.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			return dt.Stop(ctx)
		},
	})
	return dt, nil
}

// NewProviderPieceStore creates a statestore for storing metadata about pieces
// shared by the storage and retrieval providers
func NewProviderPieceStore(lc fx.Lifecycle, ds dtypes.MetadataDS) (dtypes.ProviderPieceStore, error) {
	ps, err := piecestoreimpl.NewPieceStore(namespace.Wrap(ds, datastore.NewKey("/storagemarket")))
	if err != nil {
		return nil, err
	}
	ps.OnReady(marketevents.ReadyLogger("piecestore"))
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return ps.Start(ctx)
		},
	})
	return ps, nil
}

// StagingBlockstore creates a blockstore for staging blocks for a miner
// in a storage deal, prior to sealing
func StagingBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, r repo.LockedRepo) (dtypes.StagingBlockstore, error) {
	ctx := helpers.LifecycleCtx(mctx, lc)
	stagingds, err := r.Datastore(ctx, "/staging")
	if err != nil {
		return nil, err
	}

	return blockstore.FromDatastore(stagingds), nil
}

// StagingGraphsync creates a graphsync instance which reads and writes blocks
// to the StagingBlockstore
func StagingGraphsync(parallelTransfersForStorage uint64, parallelTransfersForRetrieval uint64) func(mctx helpers.MetricsCtx, lc fx.Lifecycle, ibs dtypes.StagingBlockstore, h host.Host) dtypes.StagingGraphsync {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, ibs dtypes.StagingBlockstore, h host.Host) dtypes.StagingGraphsync {
		graphsyncNetwork := gsnet.NewFromLibp2pHost(h)
		lsys := storeutil.LinkSystemForBlockstore(ibs)
		gs := graphsync.New(helpers.LifecycleCtx(mctx, lc),
			graphsyncNetwork,
			lsys,
			graphsync.RejectAllRequestsByDefault(),
			graphsync.MaxInProgressIncomingRequests(parallelTransfersForRetrieval),
			graphsync.MaxInProgressOutgoingRequests(parallelTransfersForStorage),
			graphsyncimpl.MaxLinksPerIncomingRequests(config.MaxTraversalLinks),
			graphsyncimpl.MaxLinksPerOutgoingRequests(config.MaxTraversalLinks))

		graphsyncStats(mctx, lc, gs)

		return gs
	}
}

func SetupBlockProducer(lc fx.Lifecycle, ds dtypes.MetadataDS, api v1api.FullNode, epp gen.WinningPoStProver, sf *slashfilter.SlashFilter, j journal.Journal) (*lotusminer.Miner, error) {
	minerAddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	m := lotusminer.NewMiner(api, epp, minerAddr, sf, j)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := m.Start(ctx); err != nil {
				return err
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return m.Stop(ctx)
		},
	})

	return m, nil
}

func NewStorageAsk(ctx helpers.MetricsCtx, fapi v1api.FullNode, ds dtypes.MetadataDS, minerAddress dtypes.MinerAddress, spn storagemarket.StorageProviderNode) (*storedask.StoredAsk, error) {

	mi, err := fapi.StateMinerInfo(ctx, address.Address(minerAddress), types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	providerDs := namespace.Wrap(ds, datastore.NewKey("/deals/provider"))
	// legacy this was mistake where this key was place -- so we move the legacy key if need be
	err = shared.MoveKey(providerDs, "/latest-ask", "/storage-ask/latest")
	if err != nil {
		return nil, err
	}
	return storedask.NewStoredAsk(namespace.Wrap(providerDs, datastore.NewKey("/storage-ask")), datastore.NewKey("latest"), spn, address.Address(minerAddress),
		storagemarket.MaxPieceSize(abi.PaddedPieceSize(mi.SectorSize)))
}

func BasicDealFilter(cfg config.DealmakingConfig, user dtypes.StorageDealFilter) func(onlineOk dtypes.ConsiderOnlineStorageDealsConfigFunc,
	offlineOk dtypes.ConsiderOfflineStorageDealsConfigFunc,
	verifiedOk dtypes.ConsiderVerifiedStorageDealsConfigFunc,
	unverifiedOk dtypes.ConsiderUnverifiedStorageDealsConfigFunc,
	blocklistFunc dtypes.StorageDealPieceCidBlocklistConfigFunc,
	expectedSealTimeFunc dtypes.GetExpectedSealDurationFunc,
	startDelay dtypes.GetMaxDealStartDelayFunc,
	spn storagemarket.StorageProviderNode,
	r repo.LockedRepo,
) dtypes.StorageDealFilter {
	return func(onlineOk dtypes.ConsiderOnlineStorageDealsConfigFunc,
		offlineOk dtypes.ConsiderOfflineStorageDealsConfigFunc,
		verifiedOk dtypes.ConsiderVerifiedStorageDealsConfigFunc,
		unverifiedOk dtypes.ConsiderUnverifiedStorageDealsConfigFunc,
		blocklistFunc dtypes.StorageDealPieceCidBlocklistConfigFunc,
		expectedSealTimeFunc dtypes.GetExpectedSealDurationFunc,
		startDelay dtypes.GetMaxDealStartDelayFunc,
		spn storagemarket.StorageProviderNode,
		r repo.LockedRepo,
	) dtypes.StorageDealFilter {

		return func(ctx context.Context, deal storagemarket.MinerDeal) (bool, string, error) {
			b, err := onlineOk()
			if err != nil {
				return false, "miner error", err
			}

			if deal.Ref != nil && deal.Ref.TransferType != storagemarket.TTManual && !b {
				log.Warnf("online storage deal consideration disabled; rejecting storage deal proposal from client: %s", deal.Client.String())
				return false, "miner is not considering online storage deals", nil
			}

			b, err = offlineOk()
			if err != nil {
				return false, "miner error", err
			}

			if deal.Ref != nil && deal.Ref.TransferType == storagemarket.TTManual && !b {
				log.Warnf("offline storage deal consideration disabled; rejecting storage deal proposal from client: %s", deal.Client.String())
				return false, "miner is not accepting offline storage deals", nil
			}

			b, err = verifiedOk()
			if err != nil {
				return false, "miner error", err
			}

			if deal.Proposal.VerifiedDeal && !b {
				log.Warnf("verified storage deal consideration disabled; rejecting storage deal proposal from client: %s", deal.Client.String())
				return false, "miner is not accepting verified storage deals", nil
			}

			b, err = unverifiedOk()
			if err != nil {
				return false, "miner error", err
			}

			if !deal.Proposal.VerifiedDeal && !b {
				log.Warnf("unverified storage deal consideration disabled; rejecting storage deal proposal from client: %s", deal.Client.String())
				return false, "miner is not accepting unverified storage deals", nil
			}

			blocklist, err := blocklistFunc()
			if err != nil {
				return false, "miner error", err
			}

			for idx := range blocklist {
				if deal.Proposal.PieceCID.Equals(blocklist[idx]) {
					log.Warnf("piece CID in proposal %s is blocklisted; rejecting storage deal proposal from client: %s", deal.Proposal.PieceCID, deal.Client.String())
					return false, fmt.Sprintf("miner has blocklisted piece CID %s", deal.Proposal.PieceCID), nil
				}
			}

			sealDuration, err := expectedSealTimeFunc()
			if err != nil {
				return false, "miner error", err
			}

			sealEpochs := sealDuration / (time.Duration(build.BlockDelaySecs) * time.Second)
			_, ht, err := spn.GetChainHead(ctx)
			if err != nil {
				return false, "failed to get chain head", err
			}
			earliest := abi.ChainEpoch(sealEpochs) + ht
			if deal.Proposal.StartEpoch < earliest {
				log.Warnw("proposed deal would start before sealing can be completed; rejecting storage deal proposal from client", "piece_cid", deal.Proposal.PieceCID, "client", deal.Client.String(), "seal_duration", sealDuration, "earliest", earliest, "curepoch", ht)
				return false, fmt.Sprintf("cannot seal a sector before %s", deal.Proposal.StartEpoch), nil
			}

			sd, err := startDelay()
			if err != nil {
				return false, "miner error", err
			}

			dir := filepath.Join(r.Path(), StagingAreaDirName)
			diskUsageBytes, err := r.DiskUsage(dir)
			if err != nil {
				return false, "miner error", err
			}

			if cfg.MaxStagingDealsBytes != 0 && diskUsageBytes >= cfg.MaxStagingDealsBytes {
				log.Errorw("proposed deal rejected because there are too many deals in the staging area at the moment", "MaxStagingDealsBytes", cfg.MaxStagingDealsBytes, "DiskUsageBytes", diskUsageBytes)
				return false, "cannot accept deal as miner is overloaded at the moment - there are too many staging deals being processed", nil
			}

			// Reject if it's more than 7 days in the future
			// TODO: read from cfg
			maxStartEpoch := earliest + abi.ChainEpoch(uint64(sd.Seconds())/build.BlockDelaySecs)
			if deal.Proposal.StartEpoch > maxStartEpoch {
				return false, fmt.Sprintf("deal start epoch is too far in the future: %s > %s", deal.Proposal.StartEpoch, maxStartEpoch), nil
			}

			if user != nil {
				return user(ctx, deal)
			}

			return true, "", nil
		}
	}
}

func StorageProvider(minerAddress dtypes.MinerAddress,
	storedAsk *storedask.StoredAsk,
	h host.Host, ds dtypes.MetadataDS,
	r repo.LockedRepo,
	pieceStore dtypes.ProviderPieceStore,
	dataTransfer dtypes.ProviderDataTransfer,
	spn storagemarket.StorageProviderNode,
	df dtypes.StorageDealFilter,
	dsw *dagstore.Wrapper,
) (storagemarket.StorageProvider, error) {
	net := smnet.NewFromLibp2pHost(h)

	dir := filepath.Join(r.Path(), StagingAreaDirName)

	// migrate temporary files that were created directly under the repo, by
	// moving them to the new directory and symlinking them.
	oldDir := r.Path()
	if err := migrateDealStaging(oldDir, dir); err != nil {
		return nil, xerrors.Errorf("failed to make deal staging directory %w", err)
	}

	store, err := piecefilestore.NewLocalFileStore(piecefilestore.OsPath(dir))
	if err != nil {
		return nil, err
	}

	opt := storageimpl.CustomDealDecisionLogic(storageimpl.DealDeciderFunc(df))

	return storageimpl.NewProvider(
		net,
		namespace.Wrap(ds, datastore.NewKey("/deals/provider")),
		store,
		dsw,
		pieceStore,
		dataTransfer,
		spn,
		address.Address(minerAddress),
		storedAsk,
		opt,
	)
}

func RetrievalDealFilter(userFilter dtypes.RetrievalDealFilter) func(onlineOk dtypes.ConsiderOnlineRetrievalDealsConfigFunc,
	offlineOk dtypes.ConsiderOfflineRetrievalDealsConfigFunc) dtypes.RetrievalDealFilter {
	return func(onlineOk dtypes.ConsiderOnlineRetrievalDealsConfigFunc,
		offlineOk dtypes.ConsiderOfflineRetrievalDealsConfigFunc) dtypes.RetrievalDealFilter {
		return func(ctx context.Context, state retrievalmarket.ProviderDealState) (bool, string, error) {
			b, err := onlineOk()
			if err != nil {
				return false, "miner error", err
			}

			if !b {
				log.Warn("online retrieval deal consideration disabled; rejecting retrieval deal proposal from client")
				return false, "miner is not accepting online retrieval deals", nil
			}

			b, err = offlineOk()
			if err != nil {
				return false, "miner error", err
			}

			if !b {
				log.Info("offline retrieval has not been implemented yet")
			}

			if userFilter != nil {
				return userFilter(ctx, state)
			}

			return true, "", nil
		}
	}
}

func RetrievalNetwork(h host.Host) rmnet.RetrievalMarketNetwork {
	return rmnet.NewFromLibp2pHost(h)
}

// RetrievalPricingFunc configures the pricing function to use for retrieval deals.
func RetrievalPricingFunc(cfg config.DealmakingConfig) func(_ dtypes.ConsiderOnlineRetrievalDealsConfigFunc,
	_ dtypes.ConsiderOfflineRetrievalDealsConfigFunc) dtypes.RetrievalPricingFunc {

	return func(_ dtypes.ConsiderOnlineRetrievalDealsConfigFunc,
		_ dtypes.ConsiderOfflineRetrievalDealsConfigFunc) dtypes.RetrievalPricingFunc {
		if cfg.RetrievalPricing.Strategy == config.RetrievalPricingExternalMode {
			return pricing.ExternalRetrievalPricingFunc(cfg.RetrievalPricing.External.Path)
		}

		return retrievalimpl.DefaultPricingFunc(cfg.RetrievalPricing.Default.VerifiedDealsFreeTransfer)
	}
}

// RetrievalProvider creates a new retrieval provider attached to the provider blockstore
func RetrievalProvider(
	maddr dtypes.MinerAddress,
	adapter retrievalmarket.RetrievalProviderNode,
	sa retrievalmarket.SectorAccessor,
	netwk rmnet.RetrievalMarketNetwork,
	ds dtypes.MetadataDS,
	pieceStore dtypes.ProviderPieceStore,
	dt dtypes.ProviderDataTransfer,
	pricingFnc dtypes.RetrievalPricingFunc,
	userFilter dtypes.RetrievalDealFilter,
	dagStore *dagstore.Wrapper,
) (retrievalmarket.RetrievalProvider, error) {
	opt := retrievalimpl.DealDeciderOpt(retrievalimpl.DealDecider(userFilter))
	return retrievalimpl.NewProvider(
		address.Address(maddr),
		adapter,
		sa,
		netwk,
		pieceStore,
		dagStore,
		dt,
		namespace.Wrap(ds, datastore.NewKey("/retrievals/provider")),
		retrievalimpl.RetrievalPricingFunc(pricingFnc),
		opt,
	)
}

var WorkerCallsPrefix = datastore.NewKey("/worker/calls")
var ManagerWorkPrefix = datastore.NewKey("/stmgr/calls")

func LocalStorage(mctx helpers.MetricsCtx, lc fx.Lifecycle, ls stores.LocalStorage, si stores.SectorIndex, urls stores.URLs) (*stores.Local, error) {
	ctx := helpers.LifecycleCtx(mctx, lc)
	return stores.NewLocal(ctx, ls, si, urls)
}

func RemoteStorage(lstor *stores.Local, si stores.SectorIndex, sa sectorstorage.StorageAuth, sc sectorstorage.SealerConfig) *stores.Remote {
	return stores.NewRemote(lstor, si, http.Header(sa), sc.ParallelFetchLimit, &stores.DefaultPartialFileHandler{})
}

func SectorStorage(mctx helpers.MetricsCtx, lc fx.Lifecycle, lstor *stores.Local, stor *stores.Remote, ls stores.LocalStorage, si stores.SectorIndex, sc sectorstorage.SealerConfig, ds dtypes.MetadataDS) (*sectorstorage.Manager, error) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	wsts := statestore.New(namespace.Wrap(ds, WorkerCallsPrefix))
	smsts := statestore.New(namespace.Wrap(ds, ManagerWorkPrefix))

	sst, err := sectorstorage.New(ctx, lstor, stor, ls, si, sc, wsts, smsts)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: sst.Close,
	})

	return sst, nil
}

func StorageAuth(ctx helpers.MetricsCtx, ca v0api.Common) (sectorstorage.StorageAuth, error) {
	token, err := ca.AuthNew(ctx, []auth.Permission{"admin"})
	if err != nil {
		return nil, xerrors.Errorf("creating storage auth header: %w", err)
	}

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+string(token))
	return sectorstorage.StorageAuth(headers), nil
}

func StorageAuthWithURL(apiInfo string) func(ctx helpers.MetricsCtx, ca v0api.Common) (sectorstorage.StorageAuth, error) {
	return func(ctx helpers.MetricsCtx, ca v0api.Common) (sectorstorage.StorageAuth, error) {
		s := strings.Split(apiInfo, ":")
		if len(s) != 2 {
			return nil, errors.New("unexpected format of `apiInfo`")
		}
		headers := http.Header{}
		headers.Add("Authorization", "Bearer "+s[0])
		return sectorstorage.StorageAuth(headers), nil
	}
}

func NewConsiderOnlineStorageDealsConfigFunc(r repo.LockedRepo) (dtypes.ConsiderOnlineStorageDealsConfigFunc, error) {
	return func() (out bool, err error) {
		err = readCfg(r, func(cfg *config.StorageMiner) {
			out = cfg.Dealmaking.ConsiderOnlineStorageDeals
		})
		return
	}, nil
}

func NewSetConsideringOnlineStorageDealsFunc(r repo.LockedRepo) (dtypes.SetConsiderOnlineStorageDealsConfigFunc, error) {
	return func(b bool) (err error) {
		err = mutateCfg(r, func(cfg *config.StorageMiner) {
			cfg.Dealmaking.ConsiderOnlineStorageDeals = b
		})
		return
	}, nil
}

func NewConsiderOnlineRetrievalDealsConfigFunc(r repo.LockedRepo) (dtypes.ConsiderOnlineRetrievalDealsConfigFunc, error) {
	return func() (out bool, err error) {
		err = readCfg(r, func(cfg *config.StorageMiner) {
			out = cfg.Dealmaking.ConsiderOnlineRetrievalDeals
		})
		return
	}, nil
}

func NewSetConsiderOnlineRetrievalDealsConfigFunc(r repo.LockedRepo) (dtypes.SetConsiderOnlineRetrievalDealsConfigFunc, error) {
	return func(b bool) (err error) {
		err = mutateCfg(r, func(cfg *config.StorageMiner) {
			cfg.Dealmaking.ConsiderOnlineRetrievalDeals = b
		})
		return
	}, nil
}

func NewStorageDealPieceCidBlocklistConfigFunc(r repo.LockedRepo) (dtypes.StorageDealPieceCidBlocklistConfigFunc, error) {
	return func() (out []cid.Cid, err error) {
		err = readCfg(r, func(cfg *config.StorageMiner) {
			out = cfg.Dealmaking.PieceCidBlocklist
		})
		return
	}, nil
}

func NewSetStorageDealPieceCidBlocklistConfigFunc(r repo.LockedRepo) (dtypes.SetStorageDealPieceCidBlocklistConfigFunc, error) {
	return func(blocklist []cid.Cid) (err error) {
		err = mutateCfg(r, func(cfg *config.StorageMiner) {
			cfg.Dealmaking.PieceCidBlocklist = blocklist
		})
		return
	}, nil
}

func NewConsiderOfflineStorageDealsConfigFunc(r repo.LockedRepo) (dtypes.ConsiderOfflineStorageDealsConfigFunc, error) {
	return func() (out bool, err error) {
		err = readCfg(r, func(cfg *config.StorageMiner) {
			out = cfg.Dealmaking.ConsiderOfflineStorageDeals
		})
		return
	}, nil
}

func NewSetConsideringOfflineStorageDealsFunc(r repo.LockedRepo) (dtypes.SetConsiderOfflineStorageDealsConfigFunc, error) {
	return func(b bool) (err error) {
		err = mutateCfg(r, func(cfg *config.StorageMiner) {
			cfg.Dealmaking.ConsiderOfflineStorageDeals = b
		})
		return
	}, nil
}

func NewConsiderOfflineRetrievalDealsConfigFunc(r repo.LockedRepo) (dtypes.ConsiderOfflineRetrievalDealsConfigFunc, error) {
	return func() (out bool, err error) {
		err = readCfg(r, func(cfg *config.StorageMiner) {
			out = cfg.Dealmaking.ConsiderOfflineRetrievalDeals
		})
		return
	}, nil
}

func NewSetConsiderOfflineRetrievalDealsConfigFunc(r repo.LockedRepo) (dtypes.SetConsiderOfflineRetrievalDealsConfigFunc, error) {
	return func(b bool) (err error) {
		err = mutateCfg(r, func(cfg *config.StorageMiner) {
			cfg.Dealmaking.ConsiderOfflineRetrievalDeals = b
		})
		return
	}, nil
}

func NewConsiderVerifiedStorageDealsConfigFunc(r repo.LockedRepo) (dtypes.ConsiderVerifiedStorageDealsConfigFunc, error) {
	return func() (out bool, err error) {
		err = readCfg(r, func(cfg *config.StorageMiner) {
			out = cfg.Dealmaking.ConsiderVerifiedStorageDeals
		})
		return
	}, nil
}

func NewSetConsideringVerifiedStorageDealsFunc(r repo.LockedRepo) (dtypes.SetConsiderVerifiedStorageDealsConfigFunc, error) {
	return func(b bool) (err error) {
		err = mutateCfg(r, func(cfg *config.StorageMiner) {
			cfg.Dealmaking.ConsiderVerifiedStorageDeals = b
		})
		return
	}, nil
}

func NewConsiderUnverifiedStorageDealsConfigFunc(r repo.LockedRepo) (dtypes.ConsiderUnverifiedStorageDealsConfigFunc, error) {
	return func() (out bool, err error) {
		err = readCfg(r, func(cfg *config.StorageMiner) {
			out = cfg.Dealmaking.ConsiderUnverifiedStorageDeals
		})
		return
	}, nil
}

func NewSetConsideringUnverifiedStorageDealsFunc(r repo.LockedRepo) (dtypes.SetConsiderUnverifiedStorageDealsConfigFunc, error) {
	return func(b bool) (err error) {
		err = mutateCfg(r, func(cfg *config.StorageMiner) {
			cfg.Dealmaking.ConsiderUnverifiedStorageDeals = b
		})
		return
	}, nil
}

func NewSetSealConfigFunc(r repo.LockedRepo) (dtypes.SetSealingConfigFunc, error) {
	return func(cfg sealiface.Config) (err error) {
		err = mutateCfg(r, func(c *config.StorageMiner) {
			c.Sealing = config.SealingConfig{
				MaxWaitDealsSectors:             cfg.MaxWaitDealsSectors,
				MaxSealingSectors:               cfg.MaxSealingSectors,
				MaxSealingSectorsForDeals:       cfg.MaxSealingSectorsForDeals,
				CommittedCapacitySectorLifetime: config.Duration(cfg.CommittedCapacitySectorLifetime),
				WaitDealsDelay:                  config.Duration(cfg.WaitDealsDelay),
				AlwaysKeepUnsealedCopy:          cfg.AlwaysKeepUnsealedCopy,
				FinalizeEarly:                   cfg.FinalizeEarly,

				CollateralFromMinerBalance: cfg.CollateralFromMinerBalance,
				AvailableBalanceBuffer:     types.FIL(cfg.AvailableBalanceBuffer),
				DisableCollateralFallback:  cfg.DisableCollateralFallback,

				BatchPreCommits:     cfg.BatchPreCommits,
				MaxPreCommitBatch:   cfg.MaxPreCommitBatch,
				PreCommitBatchWait:  config.Duration(cfg.PreCommitBatchWait),
				PreCommitBatchSlack: config.Duration(cfg.PreCommitBatchSlack),

				AggregateCommits:           cfg.AggregateCommits,
				MinCommitBatch:             cfg.MinCommitBatch,
				MaxCommitBatch:             cfg.MaxCommitBatch,
				CommitBatchWait:            config.Duration(cfg.CommitBatchWait),
				CommitBatchSlack:           config.Duration(cfg.CommitBatchSlack),
				AggregateAboveBaseFee:      types.FIL(cfg.AggregateAboveBaseFee),
				BatchPreCommitAboveBaseFee: types.FIL(cfg.BatchPreCommitAboveBaseFee),

				TerminateBatchMax:  cfg.TerminateBatchMax,
				TerminateBatchMin:  cfg.TerminateBatchMin,
				TerminateBatchWait: config.Duration(cfg.TerminateBatchWait),
			}
		})
		return
	}, nil
}

func ToSealingConfig(cfg *config.StorageMiner) sealiface.Config {
	return sealiface.Config{
		MaxWaitDealsSectors:             cfg.Sealing.MaxWaitDealsSectors,
		MaxSealingSectors:               cfg.Sealing.MaxSealingSectors,
		MaxSealingSectorsForDeals:       cfg.Sealing.MaxSealingSectorsForDeals,
		CommittedCapacitySectorLifetime: time.Duration(cfg.Sealing.CommittedCapacitySectorLifetime),
		WaitDealsDelay:                  time.Duration(cfg.Sealing.WaitDealsDelay),
		AlwaysKeepUnsealedCopy:          cfg.Sealing.AlwaysKeepUnsealedCopy,
		FinalizeEarly:                   cfg.Sealing.FinalizeEarly,

		CollateralFromMinerBalance: cfg.Sealing.CollateralFromMinerBalance,
		AvailableBalanceBuffer:     types.BigInt(cfg.Sealing.AvailableBalanceBuffer),
		DisableCollateralFallback:  cfg.Sealing.DisableCollateralFallback,

		BatchPreCommits:     cfg.Sealing.BatchPreCommits,
		MaxPreCommitBatch:   cfg.Sealing.MaxPreCommitBatch,
		PreCommitBatchWait:  time.Duration(cfg.Sealing.PreCommitBatchWait),
		PreCommitBatchSlack: time.Duration(cfg.Sealing.PreCommitBatchSlack),

		AggregateCommits:           cfg.Sealing.AggregateCommits,
		MinCommitBatch:             cfg.Sealing.MinCommitBatch,
		MaxCommitBatch:             cfg.Sealing.MaxCommitBatch,
		CommitBatchWait:            time.Duration(cfg.Sealing.CommitBatchWait),
		CommitBatchSlack:           time.Duration(cfg.Sealing.CommitBatchSlack),
		AggregateAboveBaseFee:      types.BigInt(cfg.Sealing.AggregateAboveBaseFee),
		BatchPreCommitAboveBaseFee: types.BigInt(cfg.Sealing.BatchPreCommitAboveBaseFee),

		TerminateBatchMax:  cfg.Sealing.TerminateBatchMax,
		TerminateBatchMin:  cfg.Sealing.TerminateBatchMin,
		TerminateBatchWait: time.Duration(cfg.Sealing.TerminateBatchWait),

		StartEpochSealingBuffer: abi.ChainEpoch(cfg.Dealmaking.StartEpochSealingBuffer),
	}
}

func NewGetSealConfigFunc(r repo.LockedRepo) (dtypes.GetSealingConfigFunc, error) {
	return func() (out sealiface.Config, err error) {
		err = readCfg(r, func(cfg *config.StorageMiner) {
			out = ToSealingConfig(cfg)
		})
		return
	}, nil
}

func NewSetExpectedSealDurationFunc(r repo.LockedRepo) (dtypes.SetExpectedSealDurationFunc, error) {
	return func(delay time.Duration) (err error) {
		err = mutateCfg(r, func(cfg *config.StorageMiner) {
			cfg.Dealmaking.ExpectedSealDuration = config.Duration(delay)
		})
		return
	}, nil
}

func NewGetExpectedSealDurationFunc(r repo.LockedRepo) (dtypes.GetExpectedSealDurationFunc, error) {
	return func() (out time.Duration, err error) {
		err = readCfg(r, func(cfg *config.StorageMiner) {
			out = time.Duration(cfg.Dealmaking.ExpectedSealDuration)
		})
		return
	}, nil
}

func NewSetMaxDealStartDelayFunc(r repo.LockedRepo) (dtypes.SetMaxDealStartDelayFunc, error) {
	return func(delay time.Duration) (err error) {
		err = mutateCfg(r, func(cfg *config.StorageMiner) {
			cfg.Dealmaking.MaxDealStartDelay = config.Duration(delay)
		})
		return
	}, nil
}

func NewGetMaxDealStartDelayFunc(r repo.LockedRepo) (dtypes.GetMaxDealStartDelayFunc, error) {
	return func() (out time.Duration, err error) {
		err = readCfg(r, func(cfg *config.StorageMiner) {
			out = time.Duration(cfg.Dealmaking.MaxDealStartDelay)
		})
		return
	}, nil
}

func readCfg(r repo.LockedRepo, accessor func(*config.StorageMiner)) error {
	raw, err := r.Config()
	if err != nil {
		return err
	}

	cfg, ok := raw.(*config.StorageMiner)
	if !ok {
		return xerrors.New("expected address of config.StorageMiner")
	}

	accessor(cfg)

	return nil
}

func mutateCfg(r repo.LockedRepo, mutator func(*config.StorageMiner)) error {
	var typeErr error

	setConfigErr := r.SetConfig(func(raw interface{}) {
		cfg, ok := raw.(*config.StorageMiner)
		if !ok {
			typeErr = errors.New("expected miner config")
			return
		}

		mutator(cfg)
	})

	return multierr.Combine(typeErr, setConfigErr)
}

func migrateDealStaging(oldPath, newPath string) error {
	dirInfo, err := os.Stat(newPath)
	if err == nil {
		if !dirInfo.IsDir() {
			return xerrors.Errorf("%s is not a directory", newPath)
		}
		// The newPath exists already, below migration has already occurred.
		return nil
	}

	// if the directory doesn't exist, create it
	if os.IsNotExist(err) {
		if err := os.MkdirAll(newPath, 0755); err != nil {
			return xerrors.Errorf("failed to mk directory %s for deal staging: %w", newPath, err)
		}
	} else { // if we failed for other reasons, abort.
		return err
	}

	// if this is the first time we created the directory, symlink all staged deals into it. "Migration"
	// get a list of files in the miner repo
	dirEntries, err := os.ReadDir(oldPath)
	if err != nil {
		return xerrors.Errorf("failed to list directory %s for deal staging: %w", oldPath, err)
	}

	for _, entry := range dirEntries {
		// ignore directories, they are not the deals.
		if entry.IsDir() {
			continue
		}
		// the FileStore from fil-storage-market creates temporary staged deal files with the pattern "fstmp"
		// https://github.com/filecoin-project/go-fil-markets/blob/00ff81e477d846ac0cb58a0c7d1c2e9afb5ee1db/filestore/filestore.go#L69
		name := entry.Name()
		if strings.Contains(name, "fstmp") {
			// from the miner repo
			oldPath := filepath.Join(oldPath, name)
			// to its subdir "deal-staging"
			newPath := filepath.Join(newPath, name)
			// create a symbolic link in the new deal staging directory to preserve existing staged deals.
			// all future staged deals will be created here.
			if err := os.Rename(oldPath, newPath); err != nil {
				return xerrors.Errorf("failed to move %s to %s: %w", oldPath, newPath, err)
			}
			if err := os.Symlink(newPath, oldPath); err != nil {
				return xerrors.Errorf("failed to symlink %s to %s: %w", oldPath, newPath, err)
			}
			log.Infow("symlinked staged deal", "from", oldPath, "to", newPath)
		}
	}
	return nil
}

func ExtractEnabledMinerSubsystems(cfg config.MinerSubsystemConfig) (res api.MinerSubsystems) {
	if cfg.EnableMining {
		res = append(res, api.SubsystemMining)
	}
	if cfg.EnableSealing {
		res = append(res, api.SubsystemSealing)
	}
	if cfg.EnableSectorStorage {
		res = append(res, api.SubsystemSectorStorage)
	}
	if cfg.EnableMarkets {
		res = append(res, api.SubsystemMarkets)
	}
	return res
}
