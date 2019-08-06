package modules

import (
	"context"
	"github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	"github.com/libp2p/go-libp2p-core/routing"
	"path/filepath"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-merkledag"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/mitchellh/go-homedir"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/deals"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
	"github.com/filecoin-project/go-lotus/node/repo"
	"github.com/filecoin-project/go-lotus/storage"
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
			SectorSize:  1024,
			MetadataDir: metadata,
			SealedDir:   sealed,
			StagedDir:   staging,
		}

		return sb, nil
	}
}

func SectorBuilder(mctx helpers.MetricsCtx, lc fx.Lifecycle, sbc *sectorbuilder.SectorBuilderConfig) (*sectorbuilder.SectorBuilder, error) {
	sb, err := sectorbuilder.New(sbc)
	if err != nil {
		return nil, err
	}

	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			sb.Run(ctx)
			return nil
		},
	})

	return sb, nil
}

func StorageMiner(mctx helpers.MetricsCtx, lc fx.Lifecycle, api api.FullNode, h host.Host, ds dtypes.MetadataDS, sb *sectorbuilder.SectorBuilder, w *wallet.Wallet) (*storage.Miner, error) {
	maddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	sm, err := storage.NewMiner(api, maddr, h, ds, sb, w)
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
