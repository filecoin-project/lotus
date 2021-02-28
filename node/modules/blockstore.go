package modules

import (
	"context"
	"io"

	bstore "github.com/ipfs/go-ipfs-blockstore"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
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

// StateBlockstore is a hook to overlay caches for state objects, or in the
// future, to segregate the universal blockstore into different physical state
// and chain stores.
func StateBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.UniversalBlockstore) (dtypes.StateBlockstore, error) {
	return bs, nil
}

// ChainBlockstore is a hook to overlay caches for state objects, or in the
// future, to segregate the universal blockstore into different physical state
// and chain stores.
func ChainBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.UniversalBlockstore) (dtypes.ChainBlockstore, error) {
	return bs, nil
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
