package modules

import (
	"context"
	"reflect"

	"github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	graphsync "github.com/ipfs/go-graphsync/impl"
	"github.com/ipfs/go-graphsync/ipldbridge"
	gsnet "github.com/ipfs/go-graphsync/network"
	"github.com/ipfs/go-graphsync/storeutil"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-merkledag"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/routing"
	"go.uber.org/fx"
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
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storedcounter"
	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"

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
	"github.com/filecoin-project/lotus/storage/sealing"
	"github.com/filecoin-project/lotus/storage/sealmgr"
)

func minerAddrFromDS(ds dtypes.MetadataDS) (address.Address, error) {
	maddrb, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return address.Undef, err
	}

	return address.NewFromBytes(maddrb)
}

func GetParams(sbc *sectorbuilder.Config) error {
	ssize, err := sbc.SealProofType.SectorSize()
	if err != nil {
		return err
	}

	if err := paramfetch.GetParams(build.ParametersJson(), uint64(ssize)); err != nil {
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

func SectorBuilderConfig(maddr dtypes.MinerAddress, fnapi lapi.FullNode) (*sectorbuilder.Config, error) {
	ssize, err := fnapi.StateMinerSectorSize(context.TODO(), address.Address(maddr), types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	ppt, spt, err := lapi.ProofTypeFromSectorSize(ssize)
	if err != nil {
		return nil, xerrors.Errorf("bad sector size: %w", err)
	}

	sb := &sectorbuilder.Config{
		SealProofType: spt,
		PoStProofType: ppt,
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
	sc := storedcounter.New(ds, datastore.NewKey("/storage/nextid"))
	return &sidsc{sc}
}

func StorageMiner(mctx helpers.MetricsCtx, lc fx.Lifecycle, api lapi.FullNode, h host.Host, ds dtypes.MetadataDS, sealer sealmgr.Manager, sc sealing.SectorIDCounter, tktFn sealing.TicketFn) (*storage.Miner, error) {
	maddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	ctx := helpers.LifecycleCtx(mctx, lc)

	worker, err := api.StateMinerWorker(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	ppt, _, err := lapi.ProofTypeFromSectorSize(sealer.SectorSize())
	if err != nil {
		return nil, xerrors.Errorf("bad sector size: %w", err)
	}

	fps := storage.NewFPoStScheduler(api, sealer, maddr, worker, ppt)

	sm, err := storage.NewMiner(api, maddr, worker, h, ds, sealer, sc, tktFn)
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
			m.Start()
			return nil
		},
		OnStop: func(context.Context) error {
			m.Stop()
			return nil
		},
	})
}

func HandleDeals(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, h storagemarket.StorageProvider) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			h.Start(ctx)
			return nil
		},
		OnStop: func(context.Context) error {
			h.Stop()
			return nil
		},
	})
}

// RegisterProviderValidator is an initialization hook that registers the provider
// request validator with the data transfer module as the validator for
// StorageDataTransferVoucher types
func RegisterProviderValidator(mrv *requestvalidation.ProviderRequestValidator, dtm dtypes.ProviderDataTransfer) {
	if err := dtm.RegisterVoucherType(reflect.TypeOf(&requestvalidation.StorageDataTransferVoucher{}), mrv); err != nil {
		panic(err)
	}
}

// NewProviderDAGServiceDataTransfer returns a data transfer manager that just
// uses the provider's Staging DAG service for transfers
func NewProviderDAGServiceDataTransfer(h host.Host, gs dtypes.StagingGraphsync) dtypes.ProviderDataTransfer {
	return dtgraphsync.NewGraphSyncDataTransfer(h, gs)
}

// NewProviderDealStore creates a statestore for the client to store its deals
func NewProviderDealStore(ds dtypes.MetadataDS) dtypes.ProviderDealStore {
	return statestore.New(namespace.Wrap(ds, datastore.NewKey("/deals/client")))
}

// NewProviderPieceStore creates a statestore for storing metadata about pieces
// shared by the storage and retrieval providers
func NewProviderPieceStore(ds dtypes.MetadataDS) dtypes.ProviderPieceStore {
	return piecestore.NewPieceStore(ds)
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
	exch := bitswap.New(helpers.LifecycleCtx(mctx, lc), bitswapNetwork, ibs)

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
	ipldBridge := ipldbridge.NewIPLDBridge()
	loader := storeutil.LoaderForBlockstore(ibs)
	storer := storeutil.StorerForBlockstore(ibs)
	gs := graphsync.New(helpers.LifecycleCtx(mctx, lc), graphsyncNetwork, ipldBridge, loader, storer)

	return gs
}

func SetupBlockProducer(lc fx.Lifecycle, ds dtypes.MetadataDS, api lapi.FullNode, epp gen.ElectionPoStProver) (*miner.Miner, error) {
	minerAddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	m := miner.NewMiner(api, epp)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := m.Register(minerAddr); err != nil {
				return err
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return m.Unregister(ctx, minerAddr)
		},
	})

	return m, nil
}

func SealTicketGen(fapi lapi.FullNode) sealing.TicketFn {
	return func(ctx context.Context) (*lapi.SealTicket, error) {
		ts, err := fapi.ChainHead(ctx)
		if err != nil {
			return nil, xerrors.Errorf("getting head ts for SealTicket failed: %w", err)
		}

		r, err := fapi.ChainGetRandomness(ctx, ts.Key(), crypto.DomainSeparationTag_SealRandomness, ts.Height()-build.SealRandomnessLookback, nil)
		if err != nil {
			return nil, xerrors.Errorf("getting randomness for SealTicket failed: %w", err)
		}

		return &lapi.SealTicket{
			Epoch: ts.Height() - build.SealRandomnessLookback,
			Value: abi.SealRandomness(r),
		}, nil
	}
}

func NewProviderRequestValidator(deals dtypes.ProviderDealStore) *requestvalidation.ProviderRequestValidator {
	return requestvalidation.NewProviderRequestValidator(deals)
}

func StorageProvider(ctx helpers.MetricsCtx, fapi lapi.FullNode, h host.Host, ds dtypes.MetadataDS, ibs dtypes.StagingBlockstore, r repo.LockedRepo, pieceStore dtypes.ProviderPieceStore, dataTransfer dtypes.ProviderDataTransfer, spn storagemarket.StorageProviderNode) (storagemarket.StorageProvider, error) {
	store, err := piecefilestore.NewLocalFileStore(piecefilestore.OsPath(r.Path()))
	if err != nil {
		return nil, err
	}
	net := smnet.NewFromLibp2pHost(h)
	addr, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return nil, err
	}

	minerAddress, err := address.NewFromBytes(addr)
	if err != nil {
		return nil, err
	}

	ssize, err := fapi.StateMinerSectorSize(ctx, minerAddress, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	rt, _, err := lapi.ProofTypeFromSectorSize(ssize)
	if err != nil {
		return nil, err
	}

	return storageimpl.NewProvider(net, ds, ibs, store, pieceStore, dataTransfer, spn, minerAddress, rt)
}

// RetrievalProvider creates a new retrieval provider attached to the provider blockstore
func RetrievalProvider(h host.Host, miner *storage.Miner, sealer sealmgr.Manager, full lapi.FullNode, ds dtypes.MetadataDS, pieceStore dtypes.ProviderPieceStore, ibs dtypes.StagingBlockstore) (retrievalmarket.RetrievalProvider, error) {
	adapter := retrievaladapter.NewRetrievalProviderNode(miner, sealer, full)
	address, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}
	network := rmnet.NewFromLibp2pHost(h)
	return retrievalimpl.NewProvider(address, adapter, network, pieceStore, ibs, ds)
}
