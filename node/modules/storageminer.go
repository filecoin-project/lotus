package modules

import (
	"context"
	"math"
	"reflect"

	paramfetch "github.com/filecoin-project/go-paramfetch"
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
	"github.com/mitchellh/go-homedir"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	dtgraphsync "github.com/filecoin-project/go-data-transfer/impl/graphsync"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	deals "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"

	"github.com/filecoin-project/lotus/markets/retrievaladapter"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

func minerAddrFromDS(ds dtypes.MetadataDS) (address.Address, error) {
	maddrb, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return address.Undef, err
	}

	return address.NewFromBytes(maddrb)
}

func GetParams(sbc *sectorbuilder.Config) error {
	if err := paramfetch.GetParams(build.ParametersJson, sbc.SectorSize); err != nil {
		return xerrors.Errorf("fetching proof parameters: %w", err)
	}

	return nil
}

func SectorBuilderConfig(storagePath string, threads uint, noprecommit, nocommit bool) func(dtypes.MetadataDS, api.FullNode) (*sectorbuilder.Config, error) {
	return func(ds dtypes.MetadataDS, api api.FullNode) (*sectorbuilder.Config, error) {
		minerAddr, err := minerAddrFromDS(ds)
		if err != nil {
			return nil, err
		}

		ssize, err := api.StateMinerSectorSize(context.TODO(), minerAddr, nil)
		if err != nil {
			return nil, err
		}

		sp, err := homedir.Expand(storagePath)
		if err != nil {
			return nil, err
		}

		if threads > math.MaxUint8 {
			return nil, xerrors.Errorf("too many sectorbuilder threads specified: %d, max allowed: %d", threads, math.MaxUint8)
		}

		sb := &sectorbuilder.Config{
			Miner:      minerAddr,
			SectorSize: ssize,

			WorkerThreads: uint8(threads),
			NoPreCommit:   noprecommit,
			NoCommit:      nocommit,

			Dir: sp,
		}

		return sb, nil
	}
}

func StorageMiner(mctx helpers.MetricsCtx, lc fx.Lifecycle, api api.FullNode, h host.Host, ds dtypes.MetadataDS, sb sectorbuilder.Interface, tktFn storage.TicketFn) (*storage.Miner, error) {
	maddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	sm, err := storage.NewMiner(api, maddr, h, ds, sb, tktFn)
	if err != nil {
		return nil, err
	}

	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return sm.Run(ctx)
		},
		OnStop: sm.Stop,
	})

	return sm, nil
}

func HandleRetrieval(host host.Host, lc fx.Lifecycle, m retrievalmarket.RetrievalProvider) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			m.Start(host)
			return nil
		},
	})
}

func HandleDeals(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, h storagemarket.StorageProvider) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			h.Run(ctx, host)
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
func RegisterProviderValidator(mrv *deals.ProviderRequestValidator, dtm dtypes.ProviderDataTransfer) {
	if err := dtm.RegisterVoucherType(reflect.TypeOf(&deals.StorageDataTransferVoucher{}), mrv); err != nil {
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

func SetupBlockProducer(lc fx.Lifecycle, ds dtypes.MetadataDS, api api.FullNode, epp gen.ElectionPoStProver) (*miner.Miner, error) {
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

func SectorBuilder(cfg *sectorbuilder.Config, ds dtypes.MetadataDS) (*sectorbuilder.SectorBuilder, error) {
	sb, err := sectorbuilder.New(cfg, namespace.Wrap(ds, datastore.NewKey("/sectorbuilder")))
	if err != nil {
		return nil, err
	}

	return sb, nil
}

func SealTicketGen(api api.FullNode) storage.TicketFn {
	return func(ctx context.Context) (*sectorbuilder.SealTicket, error) {
		ts, err := api.ChainHead(ctx)
		if err != nil {
			return nil, xerrors.Errorf("getting head ts for SealTicket failed: %w", err)
		}

		r, err := api.ChainGetRandomness(ctx, ts.Key(), int64(ts.Height())-build.SealRandomnessLookback)
		if err != nil {
			return nil, xerrors.Errorf("getting randomness for SealTicket failed: %w", err)
		}

		var tkt [sectorbuilder.CommLen]byte
		if n := copy(tkt[:], r); n != sectorbuilder.CommLen {
			return nil, xerrors.Errorf("unexpected randomness len: %d (expected %d)", n, sectorbuilder.CommLen)
		}

		return &sectorbuilder.SealTicket{
			BlockHeight: ts.Height(),
			TicketBytes: tkt,
		}, nil
	}
}

func NewProviderRequestValidator(deals dtypes.ProviderDealStore) *storageimpl.ProviderRequestValidator {
	return storageimpl.NewProviderRequestValidator(deals)
}

func StorageProvider(ds dtypes.MetadataDS, dag dtypes.StagingDAG, dataTransfer dtypes.ProviderDataTransfer, spn storagemarket.StorageProviderNode) (storagemarket.StorageProvider, error) {
	return storageimpl.NewProvider(ds, dag, dataTransfer, spn)
}

// RetrievalProvider creates a new retrieval provider attached to the provider blockstore
func RetrievalProvider(sblks *sectorblocks.SectorBlocks, full api.FullNode) retrievalmarket.RetrievalProvider {
	adapter := retrievaladapter.NewRetrievalProviderNode(sblks, full)
	return retrievalimpl.NewProvider(adapter)
}
