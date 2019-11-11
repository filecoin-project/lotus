package modules

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"reflect"

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
	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/retrieval"
	"github.com/filecoin-project/lotus/storage"
)

func minerAddrFromDS(ds dtypes.MetadataDS) (address.Address, error) {
	maddrb, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return address.Undef, err
	}

	return address.NewFromBytes(maddrb)
}

func SectorBuilderConfig(storagePath string, threads uint) func(dtypes.MetadataDS, api.FullNode) (*sectorbuilder.Config, error) {
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

		cache := filepath.Join(sp, "cache")
		metadata := filepath.Join(sp, "meta")
		sealed := filepath.Join(sp, "sealed")
		staging := filepath.Join(sp, "staging")

		sb := &sectorbuilder.Config{
			Miner:         minerAddr,
			SectorSize:    ssize,
			WorkerThreads: uint8(threads),

			CacheDir:    cache,
			MetadataDir: metadata,
			SealedDir:   sealed,
			StagedDir:   staging,
		}

		return sb, nil
	}
}

func StorageMiner(mctx helpers.MetricsCtx, lc fx.Lifecycle, api api.FullNode, h host.Host, ds dtypes.MetadataDS, sb *sectorbuilder.SectorBuilder, tktFn storage.TicketFn) (*storage.Miner, error) {
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

// RegisterProviderValidator is an initialization hook that registers the provider
// request validator with the data transfer module as the validator for
// StorageDataTransferVoucher types
func RegisterProviderValidator(mrv *deals.ProviderRequestValidator, dtm datatransfer.ProviderDataTransfer) {
	dtm.RegisterVoucherType(reflect.TypeOf(deals.StorageDataTransferVoucher{}), mrv)
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
			if err := api.MinerRegister(ctx, minerAddr); err != nil {
				return fmt.Errorf("Failed to register miner: %s\nIf you are certain no other storage miner instance is running, try running 'lotus unregister-miner %s' and restarting the storage miner", err, minerAddr)
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Infof("Unregistering miner '%s' from full node", minerAddr)
			return api.MinerUnregister(ctx, minerAddr)
		},
	})
	return nil
}

func SectorBuilder(lc fx.Lifecycle, cfg *sectorbuilder.Config, ds dtypes.MetadataDS) (*sectorbuilder.SectorBuilder, error) {
	sb, err := sectorbuilder.New(cfg, ds)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			sb.Destroy()
			return nil
		},
	})

	return sb, nil
}

func SealTicketGen(api api.FullNode) storage.TicketFn {
	return func(ctx context.Context) (*sectorbuilder.SealTicket, error) {
		ts, err := api.ChainHead(ctx)
		if err != nil {
			return nil, xerrors.Errorf("getting head ts for SealTicket failed: %w", err)
		}

		r, err := api.ChainGetRandomness(ctx, ts, nil, build.SealRandomnessLookback)
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
