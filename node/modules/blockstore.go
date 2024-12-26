package modules

import (
	"context"

	bstore "github.com/ipfs/boxo/blockstore"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/blockstore/splitstore"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

// UniversalBlockstore returns a single universal blockstore that stores both
// chain data and state data. It can be backed by a blockstore directly
// (e.g. Badger), or by a Splitstore.
func UniversalBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, r repo.LockedRepo) (dtypes.UniversalBlockstore, error) {
	bs, closer, err := r.Blockstore(helpers.LifecycleCtx(mctx, lc), repo.UniversalBlockstore)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return closer()
		},
	})

	return bs, err
}

func MemoryBlockstore() dtypes.UniversalBlockstore {
	return blockstore.NewMemory()
}

func DiscardColdBlockstore(lc fx.Lifecycle, bs dtypes.UniversalBlockstore) (dtypes.ColdBlockstore, error) {
	return blockstore.NewDiscardStore(bs), nil
}

func BadgerHotBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, r repo.LockedRepo) (dtypes.HotBlockstore, error) {
	bs, closer, err := r.Blockstore(helpers.LifecycleCtx(mctx, lc), repo.HotBlockstore)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return closer()
		},
	})
	return bs, nil
}

func SplitBlockstore(cfg *config.Chainstore) func(lc fx.Lifecycle, r repo.LockedRepo, ds dtypes.MetadataDS, cold dtypes.ColdBlockstore, hot dtypes.HotBlockstore) (dtypes.SplitBlockstore, func() error, error) {
	return func(lc fx.Lifecycle, r repo.LockedRepo, ds dtypes.MetadataDS, cold dtypes.ColdBlockstore, hot dtypes.HotBlockstore) (dtypes.SplitBlockstore, func() error, error) {
		noop := func() error { return nil }
		path, err := r.SplitstorePath()
		if err != nil {
			return nil, noop, err
		}

		cfg := &splitstore.Config{
			MarkSetType:         cfg.Splitstore.MarkSetType,
			DiscardColdBlocks:   cfg.Splitstore.ColdStoreType == "discard",
			UniversalColdBlocks: cfg.Splitstore.ColdStoreType == "universal",
			FullWarmup:          cfg.Splitstore.FullWarmup,

			HotStoreMessageRetention:     cfg.Splitstore.HotStoreMessageRetention,
			HotStoreFullGCFrequency:      cfg.Splitstore.HotStoreFullGCFrequency,
			HotstoreMaxSpaceTarget:       cfg.Splitstore.HotStoreMaxSpaceTarget,
			HotstoreMaxSpaceThreshold:    cfg.Splitstore.HotStoreMaxSpaceThreshold,
			HotstoreMaxSpaceSafetyBuffer: cfg.Splitstore.HotstoreMaxSpaceSafetyBuffer,
		}
		ss, err := splitstore.Open(path, ds, hot, cold, cfg)
		if err != nil {
			return nil, noop, err
		}
		lc.Append(fx.Hook{
			OnStop: func(context.Context) error {
				return ss.Close()
			},
		})

		return ss, noop, nil
	}
}

func SplitBlockstoreGCReferenceProtector(_ fx.Lifecycle, s dtypes.SplitBlockstore) dtypes.GCReferenceProtector {
	return s.(dtypes.GCReferenceProtector)
}

func NoopGCReferenceProtector(_ fx.Lifecycle) dtypes.GCReferenceProtector {
	return dtypes.NoopGCReferenceProtector{}
}

func ExposedSplitBlockstore(_ fx.Lifecycle, s dtypes.SplitBlockstore) dtypes.ExposedBlockstore {
	return s.(*splitstore.SplitStore).Expose()
}

func StateFlatBlockstore(_ fx.Lifecycle, _ helpers.MetricsCtx, bs dtypes.UniversalBlockstore) (dtypes.BasicStateBlockstore, error) {
	return bs, nil
}

func StateSplitBlockstore(_ fx.Lifecycle, _ helpers.MetricsCtx, bs dtypes.SplitBlockstore) (dtypes.BasicStateBlockstore, error) {
	return bs, nil
}

func ChainFlatBlockstore(_ fx.Lifecycle, _ helpers.MetricsCtx, bs dtypes.UniversalBlockstore) (dtypes.ChainBlockstore, error) {
	return bs, nil
}

func ChainSplitBlockstore(_ fx.Lifecycle, _ helpers.MetricsCtx, bs dtypes.SplitBlockstore) (dtypes.ChainBlockstore, error) {
	return bs, nil
}

func FallbackChainBlockstore(cbs dtypes.BasicChainBlockstore) dtypes.ChainBlockstore {
	return &blockstore.FallbackStore{Blockstore: cbs}
}

func FallbackStateBlockstore(sbs dtypes.BasicStateBlockstore) dtypes.StateBlockstore {
	return &blockstore.FallbackStore{Blockstore: sbs}
}

func InitFallbackBlockstores(cbs dtypes.ChainBlockstore, sbs dtypes.StateBlockstore, rem dtypes.ChainBitswap) error {
	for _, bs := range []bstore.Blockstore{cbs, sbs} {
		if fbs, ok := bs.(*blockstore.FallbackStore); ok {
			fbs.SetFallback(rem.GetBlock)
			continue
		}
		return xerrors.Errorf("expected a FallbackStore")
	}
	return nil
}
