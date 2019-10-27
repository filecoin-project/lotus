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
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/deals"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/retrieval"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/commitment"
	"github.com/filecoin-project/lotus/storage/sector"
)

func minerAddrFromDS(ds dtypes.MetadataDS) (address.Address, error) {
	maddrb, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return address.Undef, err
	}

	return address.NewFromBytes(maddrb)
}

func SectorBuilderConfig(storagePath string) func(dtypes.MetadataDS, api.FullNode) (*sectorbuilder.SectorBuilderConfig, error) {
	return func(ds dtypes.MetadataDS, api api.FullNode) (*sectorbuilder.SectorBuilderConfig, error) {
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

		metadata := filepath.Join(sp, "meta")
		sealed := filepath.Join(sp, "sealed")
		staging := filepath.Join(sp, "staging")

		sb := &sectorbuilder.SectorBuilderConfig{
			Miner:       minerAddr,
			SectorSize:  ssize,
			MetadataDir: metadata,
			SealedDir:   sealed,
			StagedDir:   staging,
		}

		return sb, nil
	}
}

func StorageMiner(mctx helpers.MetricsCtx, lc fx.Lifecycle, api api.FullNode, h host.Host, ds dtypes.MetadataDS, secst *sector.Store, commt *commitment.Tracker) (*storage.Miner, error) {
	maddr, err := minerAddrFromDS(ds)
	if err != nil {
		return nil, err
	}

	sm, err := storage.NewMiner(api, maddr, h, ds, secst, commt)
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

func HandleDeals(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, h *deals.Provider) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			h.Run(ctx)
			host.SetStreamHandler(deals.DealProtocolID, h.HandleStream)
			host.SetStreamHandler(deals.AskProtocolID, h.HandleAskStream)
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
			log.Infof("Registering miner '%s' with full node", minerAddr)
			return api.MinerRegister(ctx, minerAddr)
		},
		OnStop: func(ctx context.Context) error {
			log.Infof("Unregistering miner '%s' from full node", minerAddr)
			return api.MinerUnregister(ctx, minerAddr)
		},
	})
	return nil
}

func SealTicketGen(api api.FullNode) sector.TicketFn {
	return func(ctx context.Context) (*sectorbuilder.SealTicket, error) {
		ts, err := api.ChainHead(ctx)
		if err != nil {
			return nil, xerrors.Errorf("getting head ts for SealTicket failed: %w", err)
		}

		r, err := api.ChainGetRandomness(ctx, ts, nil, build.PoSTChallangeTime)
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
