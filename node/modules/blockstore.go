package modules

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	lmdbbs "github.com/filecoin-project/go-bs-lmdb"
	badgerbs "github.com/filecoin-project/lotus/lib/blockstore/badger"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/store/splitstore"
	"github.com/filecoin-project/lotus/node/config"
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

func LMDBHotBlockstore(lc fx.Lifecycle, r repo.LockedRepo) (dtypes.HotBlockstore, error) {
	path, err := r.SplitstorePath()
	if err != nil {
		return nil, err
	}

	path = filepath.Join(path, "hot.lmdb")
	bs, err := lmdbbs.Open(&lmdbbs.Options{
		Path:                 path,
		InitialMmapSize:      4 << 30, // 4GiB.
		MmapGrowthStepFactor: 1.25,    // scale slower than the default of 1.5
		MmapGrowthStepMax:    4 << 30, // 4GiB
		RetryDelay:           10 * time.Microsecond,
		MaxReaders:           1024,
	})
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return bs.Close()
		}})

	hot := blockstore.WrapIDStore(bs)
	return hot, err
}

func BadgerHotBlockstore(lc fx.Lifecycle, r repo.LockedRepo) (dtypes.HotBlockstore, error) {
	path, err := r.SplitstorePath()
	if err != nil {
		return nil, err
	}

	path = filepath.Join(path, "hot.badger")
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, err
	}

	opts, err := repo.BadgerBlockstoreOptions(repo.HotBlockstore, path, r.Readonly())
	if err != nil {
		return nil, err
	}

	bs, err := badgerbs.Open(opts)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return bs.Close()
		}})

	hot := blockstore.WrapIDStore(bs)
	return hot, err
}

func SplitBlockstore(cfg *config.Blockstore) func(lc fx.Lifecycle, r repo.LockedRepo, ds dtypes.MetadataDS, cold dtypes.ColdBlockstore, hot dtypes.HotBlockstore) (dtypes.SplitBlockstore, error) {
	return func(lc fx.Lifecycle, r repo.LockedRepo, ds dtypes.MetadataDS, cold dtypes.ColdBlockstore, hot dtypes.HotBlockstore) (dtypes.SplitBlockstore, error) {
		path, err := r.SplitstorePath()
		if err != nil {
			return nil, err
		}

		ss, err := splitstore.NewSplitStore(path, ds, cold, hot, cfg.UseLMDB)
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
}

func StateFlatBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.ColdBlockstore) (dtypes.StateBlockstore, error) {
	return bs, nil
}

func StateSplitBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.SplitBlockstore) (dtypes.StateBlockstore, error) {
	return bs, nil
}

func ChainFlatBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.ColdBlockstore) (dtypes.ChainBlockstore, error) {
	return bs, nil
}

func ChainSplitBlockstore(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.SplitBlockstore) (dtypes.ChainBlockstore, error) {
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
