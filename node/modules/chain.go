package modules

import (
	"bytes"
	"context"

	"github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-car"
	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/routing"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

func ChainExchange(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, rt routing.Routing, bs dtypes.ChainGCBlockstore) dtypes.ChainExchange {
	// prefix protocol for chain bitswap
	// (so bitswap uses /chain/ipfs/bitswap/1.0.0 internally for chain sync stuff)
	bitswapNetwork := network.NewFromIpfsHost(host, rt, network.Prefix("/chain"))
	exch := bitswap.New(helpers.LifecycleCtx(mctx, lc), bitswapNetwork, bs)
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return exch.Close()
		},
	})

	return exch
}

func MessagePool(lc fx.Lifecycle, sm *stmgr.StateManager, ps *pubsub.PubSub, ds dtypes.MetadataDS) (*messagepool.MessagePool, error) {
	mpp := messagepool.NewProvider(sm, ps)
	mp, err := messagepool.New(mpp, ds)
	if err != nil {
		return nil, xerrors.Errorf("constructing mpool: %w", err)
	}
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return mp.Close()
		},
	})
	return mp, nil
}

func ChainBlockstore(r repo.LockedRepo) (dtypes.ChainBlockstore, error) {
	blocks, err := r.Datastore("/blocks")
	if err != nil {
		return nil, err
	}

	bs := blockstore.NewBlockstore(blocks)
	return blockstore.NewIdStore(bs), nil
}

func ChainGCBlockstore(bs dtypes.ChainBlockstore, gcl dtypes.ChainGCLocker) dtypes.ChainGCBlockstore {
	return blockstore.NewGCBlockstore(bs, gcl)
}

func ChainBlockservice(bs dtypes.ChainBlockstore, rem dtypes.ChainExchange) dtypes.ChainBlockService {
	return blockservice.New(bs, rem)
}

func ChainStore(lc fx.Lifecycle, bs dtypes.ChainBlockstore, ds dtypes.MetadataDS, syscalls *types.VMSyscalls) *store.ChainStore {
	chain := store.NewChainStore(bs, ds, syscalls)

	if err := chain.Load(); err != nil {
		log.Warnf("loading chain state from disk: %s", err)
	}

	return chain
}

func ErrorGenesis() Genesis {
	return func() (header *types.BlockHeader, e error) {
		return nil, xerrors.New("No genesis block provided, provide the file with 'lotus daemon --genesis=[genesis file]'")
	}
}

func LoadGenesis(genBytes []byte) func(dtypes.ChainBlockstore) Genesis {
	return func(bs dtypes.ChainBlockstore) Genesis {
		return func() (header *types.BlockHeader, e error) {
			c, err := car.LoadCar(bs, bytes.NewReader(genBytes))
			if err != nil {
				return nil, err
			}
			if len(c.Roots) != 1 {
				return nil, xerrors.New("expected genesis file to have one root")
			}
			root, err := bs.Get(c.Roots[0])
			if err != nil {
				return &types.BlockHeader{}, err
			}

			return types.DecodeBlock(root.RawData())
		}
	}
}

func SetGenesis(cs *store.ChainStore, g Genesis) error {
	_, err := cs.GetGenesis()
	if err == nil {
		return nil // already set, noop
	}
	if err != datastore.ErrNotFound {
		return err
	}

	genesis, err := g()
	if err != nil {
		return err
	}

	return cs.SetGenesis(genesis)
}

func NewSyncer(lc fx.Lifecycle, sm *stmgr.StateManager, bsync *blocksync.BlockSync, h host.Host) (*chain.Syncer, error) {
	syncer, err := chain.NewSyncer(sm, bsync, h.ConnManager(), h.ID())
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			syncer.Start()
			return nil
		},
		OnStop: func(_ context.Context) error {
			syncer.Stop()
			return nil
		},
	})
	return syncer, nil
}
