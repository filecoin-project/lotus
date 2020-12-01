package modules

import (
	"context"
	"io"

	bstore "github.com/ipfs/go-ipfs-blockstore"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/store/splitstore"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

// UniversalBlockstore returns a single universal blockstore that stores both
// chain data and state data.
func UniversalBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, r repo.LockedRepo) (dtypes.UniversalBlockstore, error) {
	bs, err := r.Blockstore(helpers.LifecycleCtx(mctx, lc), repo.UniversalBlockstore)
	if err != nil {
		return nil, err
	}
	if c, ok := bs.(io.Closer); ok {
		lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				return c.Close()
			},
		})
	}
	return bs, err
}

func SplitBlockstore(lc fx.Lifecycle, r repo.LockedRepo, ds dtypes.MetadataDS, bs dtypes.UniversalBlockstore) (dtypes.SplitBlockstore, error) {
	path, err := r.SplitstorePath()
	if err != nil {
		return nil, err
	}

	ss, err := splitstore.NewSplitStore(path, ds, bs)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return ss.Close()
		},
	})

	return ss, err
}

// StateBlockstore returns the blockstore to use to store the state tree.
// StateBlockstore is a hook to overlay caches for state objects, or in the
// future, to segregate the universal blockstore into different physical state
// and chain stores.
func StateBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.SplitBlockstore) (dtypes.StateBlockstore, error) {
	sbs, err := blockstore.WrapFreecacheCache(helpers.LifecycleCtx(mctx, lc), bs, blockstore.FreecacheConfig{
		Name:           "state",
		BlockCapacity:  288 * 1024 * 1024, // 288MiB.
		ExistsCapacity: 48 * 1024 * 1024,  // 48MiB.
	})
	if err != nil {
		return nil, err
	}
	// this may end up double closing the underlying blockstore, but all
	// blockstores should be lenient or idempotent on double-close. The native
	// badger blockstore is (and unit tested).
	if c, ok := bs.(io.Closer); ok {
		lc.Append(closerStopHook(c))
	}
	return sbs, nil
}

// ChainBlockstore returns the blockstore to use for chain data (tipsets, blocks, messages).
// ChainBlockstore is a hook to overlay caches for state objects, or in the
// future, to segregate the universal blockstore into different physical state
// and chain stores.
func ChainBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.SplitBlockstore) (dtypes.ChainBlockstore, error) {
	cbs, err := blockstore.WrapFreecacheCache(helpers.LifecycleCtx(mctx, lc), bs, blockstore.FreecacheConfig{
		Name:           "chain",
		BlockCapacity:  64 * 1024 * 1024, // 64MiB.
		ExistsCapacity: 16 * 1024,        // 16MiB.
	})
	if err != nil {
		return nil, err
	}
	// this may end up double closing the underlying blockstore, but all
	// blockstores should be lenient or idempotent on double-close. The native
	// badger blockstore is (and unit tested).
	if c, ok := bs.(io.Closer); ok {
		lc.Append(closerStopHook(c))
	}
	return cbs, nil
}

func FallbackChainBlockstore(cbs dtypes.ChainBlockstore) dtypes.ChainBlockstore {
	return &blockstore.FallbackStore{Blockstore: cbs}
}

func FallbackStateBlockstore(sbs dtypes.StateBlockstore) dtypes.StateBlockstore {
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
