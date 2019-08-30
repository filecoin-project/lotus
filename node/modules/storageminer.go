package modules

import (
	"context"
	"path/filepath"

	"github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-merkledag"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/routing"
	"github.com/mitchellh/go-homedir"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/deals"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
	"github.com/filecoin-project/go-lotus/node/repo"
	"github.com/filecoin-project/go-lotus/retrieval"
	"github.com/filecoin-project/go-lotus/storage"
	"github.com/filecoin-project/go-lotus/storage/sector"
)

func minerAddrFromDS(ds dtypes.MetadataDS) (address.Address, error) {
	maddrb, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return address.Undef, err
	}

	return address.NewFromBytes(maddrb)
}

func SectorBuilderConfig(storagePath string) func(dtypes.MetadataDS) (*sectorbuilder.SectorBuilderConfig, error) {
	return func(ds dtypes.MetadataDS) (*sectorbuilder.SectorBuilderConfig, error) {
		minerAddr, err := minerAddrFromDS(ds)
		if err != nil {
			return nil, err
		}

		sp, err := homedir.Expand(storagePath)
		if err != nil {
			return nil, err
		}

		metadata := filepath.Join(sp, "meta")
		sealed := filepath.Join(sp, "sealed")
		staging := filepath.Join(sp, "staging")

		sb := &sectorbuilder.SectorBuilderConfig{
			Miner:       minerAddr,
			SectorSize:  build.SectorSize,
			MetadataDir: metadata,
			SealedDir:   sealed,
			StagedDir:   staging,
		}

		return sb, nil
	}
}

func StorageMiner(mctx helpers.MetricsCtx, lc fx.Lifecycle, api api.FullNode, h host.Host, ds dtypes.MetadataDS, secst *sector.Store) (*storage.Miner, error) {
	maddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	sm, err := storage.NewMiner(api, maddr, h, ds, secst)
	if err != nil {
		return nil, err
	}

	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return sm.Run(ctx)
		},
	})

	return sm, nil
}

func HandleRetrieval(host host.Host, lc fx.Lifecycle, m *retrieval.Miner) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			host.SetStreamHandler(retrieval.QueryProtocolID, m.HandleQueryStream)
			host.SetStreamHandler(retrieval.ProtocolID, m.HandleDealStream)
			return nil
		},
	})
}

func HandleDeals(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, h *deals.Handler) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			h.Run(ctx)
			host.SetStreamHandler(deals.ProtocolID, h.HandleStream)
			return nil
		},
		OnStop: func(context.Context) error {
			h.Stop()
			return nil
		},
	})
}

func StagingDAG(mctx helpers.MetricsCtx, lc fx.Lifecycle, r repo.LockedRepo, rt routing.Routing, h host.Host) (dtypes.StagingDAG, error) {
	stagingds, err := r.Datastore("/staging")
	if err != nil {
		return nil, err
	}

	bs := blockstore.NewBlockstore(stagingds)
	ibs := blockstore.NewIdStore(bs)

	bitswapNetwork := network.NewFromIpfsHost(h, rt)
	exch := bitswap.New(helpers.LifecycleCtx(mctx, lc), bitswapNetwork, bs)

	bsvc := blockservice.New(ibs, exch)
	dag := merkledag.NewDAGService(bsvc)

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return bsvc.Close()
		},
	})

	return dag, nil
}

func RegisterMiner(lc fx.Lifecycle, ds dtypes.MetadataDS, api api.FullNode) error {
	minerAddr, err := minerAddrFromDS(ds)
	if err != nil {
		return err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Infof("registering miner '%s' with full node", minerAddr)
			return api.MinerRegister(ctx, minerAddr)
		},
	})
	return nil
}
