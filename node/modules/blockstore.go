package modules

import (
	"context"
	"io"

	bstore "github.com/ipfs/go-ipfs-blockstore"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

// BareMonolithBlockstore returns a bare local blockstore for chain and state
// data, with direct access. In the future, both data domains may be segregated
// into individual blockstores.
func BareMonolithBlockstore(lc fx.Lifecycle, r repo.LockedRepo) (dtypes.BareMonolithBlockstore, error) {
	bs, err := r.Blockstore(repo.BlockstoreMonolith)
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

// StateBlockstore returns the blockstore to use to store the state tree.
func StateBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.BareMonolithBlockstore) (dtypes.StateBlockstore, error) {
	sbs, err := blockstore.WrapFreecacheCache(helpers.LifecycleCtx(mctx, lc), bs, blockstore.FreecacheConfig{
		Name:           "state",
		BlockCapacity:  1 << 28, // 256MiB.
		ExistsCapacity: 1 << 25, // 32MiB.
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
func ChainBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.BareMonolithBlockstore) (dtypes.ChainBlockstore, error) {
	cbs, err := blockstore.WrapFreecacheCache(helpers.LifecycleCtx(mctx, lc), bs, blockstore.FreecacheConfig{
		Name:           "chain",
		BlockCapacity:  1 << 27, // 128MiB.
		ExistsCapacity: 1 << 24, // 16MiB.
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
