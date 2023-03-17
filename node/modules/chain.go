package modules

import (
	"context"
	"time"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-libipfs/bitswap"
	"github.com/ipfs/go-libipfs/bitswap/network"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/routing"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/brossetti1/lotus/blockstore"
	"github.com/brossetti1/lotus/blockstore/splitstore"
	"github.com/brossetti1/lotus/build"
	"github.com/brossetti1/lotus/chain"
	"github.com/brossetti1/lotus/chain/beacon"
	"github.com/brossetti1/lotus/chain/consensus"
	"github.com/brossetti1/lotus/chain/consensus/filcns"
	"github.com/brossetti1/lotus/chain/exchange"
	"github.com/brossetti1/lotus/chain/gen/slashfilter"
	"github.com/brossetti1/lotus/chain/messagepool"
	"github.com/brossetti1/lotus/chain/stmgr"
	"github.com/brossetti1/lotus/chain/store"
	"github.com/brossetti1/lotus/chain/vm"
	"github.com/brossetti1/lotus/journal"
	"github.com/brossetti1/lotus/node/modules/dtypes"
	"github.com/brossetti1/lotus/node/modules/helpers"
)

// ChainBitswap uses a blockstore that bypasses all caches.
func ChainBitswap(lc fx.Lifecycle, mctx helpers.MetricsCtx, host host.Host, rt routing.Routing, bs dtypes.ExposedBlockstore) dtypes.ChainBitswap {
	// prefix protocol for chain bitswap
	// (so bitswap uses /chain/ipfs/bitswap/1.0.0 internally for chain sync stuff)
	bitswapNetwork := network.NewFromIpfsHost(host, rt, network.Prefix("/chain"))
	bitswapOptions := []bitswap.Option{bitswap.ProvideEnabled(false)}

	// Write all incoming bitswap blocks into a temporary blockstore for two
	// block times. If they validate, they'll be persisted later.
	cache := blockstore.NewTimedCacheBlockstore(2 * time.Duration(build.BlockDelaySecs) * time.Second)
	lc.Append(fx.Hook{OnStop: cache.Stop, OnStart: cache.Start})

	bitswapBs := blockstore.NewTieredBstore(bs, cache)

	// Use just exch.Close(), closing the context is not needed
	exch := bitswap.New(mctx, bitswapNetwork, bitswapBs, bitswapOptions...)
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return exch.Close()
		},
	})

	return exch
}

func ChainBlockService(bs dtypes.ExposedBlockstore, rem dtypes.ChainBitswap) dtypes.ChainBlockService {
	return blockservice.New(bs, rem)
}

func MessagePool(lc fx.Lifecycle, mctx helpers.MetricsCtx, us stmgr.UpgradeSchedule, mpp messagepool.Provider, ds dtypes.MetadataDS, nn dtypes.NetworkName, j journal.Journal, protector dtypes.GCReferenceProtector) (*messagepool.MessagePool, error) {
	mp, err := messagepool.New(helpers.LifecycleCtx(mctx, lc), mpp, ds, us, nn, j)
	if err != nil {
		return nil, xerrors.Errorf("constructing mpool: %w", err)
	}
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return mp.Close()
		},
	})
	protector.AddProtector(mp.TryForEachPendingMessage)
	return mp, nil
}

func ChainStore(lc fx.Lifecycle,
	mctx helpers.MetricsCtx,
	cbs dtypes.ChainBlockstore,
	sbs dtypes.StateBlockstore,
	ds dtypes.MetadataDS,
	basebs dtypes.BaseBlockstore,
	weight store.WeightFunc,
	us stmgr.UpgradeSchedule,
	j journal.Journal) *store.ChainStore {

	chain := store.NewChainStore(cbs, sbs, ds, weight, j)

	if err := chain.Load(helpers.LifecycleCtx(mctx, lc)); err != nil {
		log.Warnf("loading chain state from disk: %s", err)
	}

	var startHook func(context.Context) error
	if ss, ok := basebs.(*splitstore.SplitStore); ok {
		startHook = func(_ context.Context) error {
			err := ss.Start(chain, us)
			if err != nil {
				err = xerrors.Errorf("error starting splitstore: %w", err)
			}
			return err
		}
	}

	lc.Append(fx.Hook{
		OnStart: startHook,
		OnStop: func(_ context.Context) error {
			return chain.Close()
		},
	})

	return chain
}

func NetworkName(mctx helpers.MetricsCtx,
	lc fx.Lifecycle,
	cs *store.ChainStore,
	tsexec stmgr.Executor,
	syscalls vm.SyscallBuilder,
	us stmgr.UpgradeSchedule,
	_ dtypes.AfterGenesisSet) (dtypes.NetworkName, error) {
	if !build.Devnet {
		return "testnetnet", nil
	}

	ctx := helpers.LifecycleCtx(mctx, lc)

	sm, err := stmgr.NewStateManager(cs, tsexec, syscalls, us, nil, nil)
	if err != nil {
		return "", err
	}

	netName, err := stmgr.GetNetworkName(ctx, sm, cs.GetHeaviestTipSet().ParentState())
	return netName, err
}

type SyncerParams struct {
	fx.In

	Lifecycle    fx.Lifecycle
	MetadataDS   dtypes.MetadataDS
	StateManager *stmgr.StateManager
	ChainXchg    exchange.Client
	SyncMgrCtor  chain.SyncManagerCtor
	Host         host.Host
	Beacon       beacon.Schedule
	Gent         chain.Genesis
	Consensus    consensus.Consensus
}

func NewSyncer(params SyncerParams) (*chain.Syncer, error) {
	var (
		lc     = params.Lifecycle
		ds     = params.MetadataDS
		sm     = params.StateManager
		ex     = params.ChainXchg
		smCtor = params.SyncMgrCtor
		h      = params.Host
		b      = params.Beacon
	)
	syncer, err := chain.NewSyncer(ds, sm, ex, smCtor, h.ConnManager(), h.ID(), b, params.Gent, params.Consensus)
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

func NewSlashFilter(ds dtypes.MetadataDS) *slashfilter.SlashFilter {
	return slashfilter.New(ds)
}

func UpgradeSchedule() stmgr.UpgradeSchedule {
	return filcns.DefaultUpgradeSchedule()
}

func EnableStoringEvents(cs *store.ChainStore) {
	cs.StoreEvents(true)
}
