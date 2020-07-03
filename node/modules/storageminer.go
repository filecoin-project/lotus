package modules

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	graphsync "github.com/ipfs/go-graphsync/impl"
	gsnet "github.com/ipfs/go-graphsync/network"
	"github.com/ipfs/go-graphsync/storeutil"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-merkledag"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/routing"
	"go.uber.org/fx"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	dtgraphsync "github.com/filecoin-project/go-data-transfer/impl/graphsync"
	piecefilestore "github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-jsonrpc/auth"
	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/go-storedcounter"
	"github.com/filecoin-project/lotus/node/config"
	sectorstorage "github.com/filecoin-project/sector-storage"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/sector-storage/stores"
	"github.com/filecoin-project/specs-actors/actors/abi"
	sealing "github.com/filecoin-project/storage-fsm"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/markets/retrievaladapter"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage"
)

var StorageCounterDSPrefix = "/storage/nextid"

func minerAddrFromDS(ds dtypes.MetadataDS) (address.Address, error) {
	maddrb, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return address.Undef, err
	}

	return address.NewFromBytes(maddrb)
}

func GetParams(sbc *ffiwrapper.Config) error {
	ssize, err := sbc.SealProofType.SectorSize()
	if err != nil {
		return err
	}

	// If built-in assets are disabled, we expect the user to have placed the right
	// parameters in the right location on the filesystem (/var/tmp/filecoin-proof-parameters).
	if build.DisableBuiltinAssets {
		return nil
	}

	if err := paramfetch.GetParams(context.TODO(), build.ParametersJSON(), uint64(ssize)); err != nil {
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

func StorageNetworkName(ctx helpers.MetricsCtx, a lapi.FullNode) (dtypes.NetworkName, error) {
	return a.StateNetworkName(ctx)
}

func ProofsConfig(maddr dtypes.MinerAddress, fnapi lapi.FullNode) (*ffiwrapper.Config, error) {
	mi, err := fnapi.StateMinerInfo(context.TODO(), address.Address(maddr), types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	spt, err := ffiwrapper.SealProofTypeFromSectorSize(mi.SectorSize)
	if err != nil {
		return nil, xerrors.Errorf("bad sector size: %w", err)
	}

	sb := &ffiwrapper.Config{
		SealProofType: spt,
	}

	return sb, nil
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

func StorageMiner(mctx helpers.MetricsCtx, lc fx.Lifecycle, api lapi.FullNode, h host.Host, ds dtypes.MetadataDS, sealer sectorstorage.SectorManager, sc sealing.SectorIDCounter, verif ffiwrapper.Verifier) (*storage.Miner, error) {
	maddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	ctx := helpers.LifecycleCtx(mctx, lc)

	mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	worker, err := api.StateAccountKey(ctx, mi.Worker, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	fps, err := storage.NewWindowedPoStScheduler(api, sealer, sealer, maddr, worker)
	if err != nil {
		return nil, err
	}

	sm, err := storage.NewMiner(api, maddr, worker, h, ds, sealer, sc, verif)
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

func HandleRetrieval(host host.Host, lc fx.Lifecycle, m retrievalmarket.RetrievalProvider) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return m.Start()
		},
		OnStop: func(context.Context) error {
			return m.Stop()
		},
	})
}

func HandleDeals(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, h storagemarket.StorageProvider) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return h.Start(ctx)
		},
		OnStop: func(context.Context) error {
			return h.Stop()
		},
	})
}

// RegisterProviderValidator is an initialization hook that registers the provider
// request validator with the data transfer module as the validator for
// StorageDataTransferVoucher types
func RegisterProviderValidator(mrv dtypes.ProviderRequestValidator, dtm dtypes.ProviderDataTransfer) {
	if err := dtm.RegisterVoucherType(&requestvalidation.StorageDataTransferVoucher{}, (*requestvalidation.UnifiedRequestValidator)(mrv)); err != nil {
		panic(err)
	}
}

// NewProviderDAGServiceDataTransfer returns a data transfer manager that just
// uses the provider's Staging DAG service for transfers
func NewProviderDAGServiceDataTransfer(h host.Host, gs dtypes.StagingGraphsync, ds dtypes.MetadataDS) dtypes.ProviderDataTransfer {
	sc := storedcounter.New(ds, datastore.NewKey("/datatransfer/provider/counter"))
	return dtgraphsync.NewGraphSyncDataTransfer(h, gs, sc)
}

// NewProviderDealStore creates a statestore for the client to store its deals
func NewProviderDealStore(ds dtypes.MetadataDS) dtypes.ProviderDealStore {
	return statestore.New(namespace.Wrap(ds, datastore.NewKey("/deals/provider")))
}

// NewProviderPieceStore creates a statestore for storing metadata about pieces
// shared by the storage and retrieval providers
func NewProviderPieceStore(ds dtypes.MetadataDS) dtypes.ProviderPieceStore {
	return piecestore.NewPieceStore(namespace.Wrap(ds, datastore.NewKey("/storagemarket")))
}

// StagingBlockstore creates a blockstore for staging blocks for a miner
// in a storage deal, prior to sealing
func StagingBlockstore(r repo.LockedRepo) (dtypes.StagingBlockstore, error) {
	stagingds, err := r.Datastore("/staging")
	if err != nil {
		return nil, err
	}

	bs := blockstore.NewBlockstore(stagingds)
	ibs := blockstore.NewIdStore(bs)

	return ibs, nil
}

// StagingDAG is a DAGService for the StagingBlockstore
func StagingDAG(mctx helpers.MetricsCtx, lc fx.Lifecycle, ibs dtypes.StagingBlockstore, rt routing.Routing, h host.Host) (dtypes.StagingDAG, error) {

	bitswapNetwork := network.NewFromIpfsHost(h, rt)
	bitswapOptions := []bitswap.Option{bitswap.ProvideEnabled(false)}
	exch := bitswap.New(helpers.LifecycleCtx(mctx, lc), bitswapNetwork, ibs, bitswapOptions...)

	bsvc := blockservice.New(ibs, exch)
	dag := merkledag.NewDAGService(bsvc)

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return bsvc.Close()
		},
	})

	return dag, nil
}

// StagingGraphsync creates a graphsync instance which reads and writes blocks
// to the StagingBlockstore
func StagingGraphsync(mctx helpers.MetricsCtx, lc fx.Lifecycle, ibs dtypes.StagingBlockstore, h host.Host) dtypes.StagingGraphsync {
	graphsyncNetwork := gsnet.NewFromLibp2pHost(h)
	loader := storeutil.LoaderForBlockstore(ibs)
	storer := storeutil.StorerForBlockstore(ibs)
	gs := graphsync.New(helpers.LifecycleCtx(mctx, lc), graphsyncNetwork, loader, storer, graphsync.RejectAllRequestsByDefault())

	return gs
}

func SetupBlockProducer(lc fx.Lifecycle, ds dtypes.MetadataDS, api lapi.FullNode, epp gen.WinningPoStProver) (*miner.Miner, error) {
	minerAddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	m := miner.NewMiner(api, epp, minerAddr)

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

func NewProviderRequestValidator(deals dtypes.ProviderDealStore) dtypes.ProviderRequestValidator {
	return requestvalidation.NewUnifiedRequestValidator(deals, nil)
}

func NewStorageAsk(ctx helpers.MetricsCtx, fapi lapi.FullNode, ds dtypes.MetadataDS, minerAddress dtypes.MinerAddress, spn storagemarket.StorageProviderNode) (*storedask.StoredAsk, error) {

	mi, err := fapi.StateMinerInfo(ctx, address.Address(minerAddress), types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	storedAsk, err := storedask.NewStoredAsk(namespace.Wrap(ds, datastore.NewKey("/deals/provider")), datastore.NewKey("latest-ask"), spn, address.Address(minerAddress))
	if err != nil {
		return nil, err
	}
	// Hacky way to set max piece size to the sector size
	a := storedAsk.GetAsk().Ask
	err = storedAsk.SetAsk(a.Price, a.Expiry-a.Timestamp, storagemarket.MaxPieceSize(abi.PaddedPieceSize(mi.SectorSize)))
	if err != nil {
		return storedAsk, err
	}
	return storedAsk, nil
}

func StorageProvider(minerAddress dtypes.MinerAddress, ffiConfig *ffiwrapper.Config, storedAsk *storedask.StoredAsk, h host.Host, ds dtypes.MetadataDS, ibs dtypes.StagingBlockstore, r repo.LockedRepo, pieceStore dtypes.ProviderPieceStore, dataTransfer dtypes.ProviderDataTransfer, spn storagemarket.StorageProviderNode, onlineOk dtypes.ConsiderOnlineStorageDealsConfigFunc, offlineOk dtypes.ConsiderOfflineStorageDealsConfigFunc, blocklistFunc dtypes.StorageDealPieceCidBlocklistConfigFunc) (storagemarket.StorageProvider, error) {
	net := smnet.NewFromLibp2pHost(h)
	store, err := piecefilestore.NewLocalFileStore(piecefilestore.OsPath(r.Path()))
	if err != nil {
		return nil, err
	}

	opt := storageimpl.CustomDealDecisionLogic(func(ctx context.Context, deal storagemarket.MinerDeal) (bool, string, error) {
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

		return true, "", nil
	})

	p, err := storageimpl.NewProvider(net, namespace.Wrap(ds, datastore.NewKey("/deals/provider")), ibs, store, pieceStore, dataTransfer, spn, address.Address(minerAddress), ffiConfig.SealProofType, storedAsk, opt)
	if err != nil {
		return p, err
	}

	return p, nil
}

// RetrievalProvider creates a new retrieval provider attached to the provider blockstore
func RetrievalProvider(h host.Host, miner *storage.Miner, sealer sectorstorage.SectorManager, full lapi.FullNode, ds dtypes.MetadataDS, pieceStore dtypes.ProviderPieceStore, ibs dtypes.StagingBlockstore, onlineOk dtypes.ConsiderOnlineRetrievalDealsConfigFunc, offlineOk dtypes.ConsiderOfflineRetrievalDealsConfigFunc) (retrievalmarket.RetrievalProvider, error) {
	adapter := retrievaladapter.NewRetrievalProviderNode(miner, sealer, full)

	maddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	netwk := rmnet.NewFromLibp2pHost(h)

	opt := retrievalimpl.DealDeciderOpt(func(ctx context.Context, state retrievalmarket.ProviderDealState) (bool, string, error) {
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

		return true, "", nil
	})

	return retrievalimpl.NewProvider(maddr, adapter, netwk, pieceStore, ibs, namespace.Wrap(ds, datastore.NewKey("/retrievals/provider")), opt)
}

func SectorStorage(mctx helpers.MetricsCtx, lc fx.Lifecycle, ls stores.LocalStorage, si stores.SectorIndex, cfg *ffiwrapper.Config, sc sectorstorage.SealerConfig, urls sectorstorage.URLs, sa sectorstorage.StorageAuth) (*sectorstorage.Manager, error) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	sst, err := sectorstorage.New(ctx, ls, si, cfg, sc, urls, sa)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			if err := sst.Close(); err != nil {
				log.Errorf("%+v", err)
			}

			return nil
		},
	})

	return sst, nil
}

func StorageAuth(ctx helpers.MetricsCtx, ca lapi.Common) (sectorstorage.StorageAuth, error) {
	token, err := ca.AuthNew(ctx, []auth.Permission{"admin"})
	if err != nil {
		return nil, xerrors.Errorf("creating storage auth header: %w", err)
	}

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+string(token))
	return sectorstorage.StorageAuth(headers), nil
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
			typeErr = errors.New("expected storage miner config")
			return
		}

		mutator(cfg)
	})

	return multierr.Combine(typeErr, setConfigErr)
}
