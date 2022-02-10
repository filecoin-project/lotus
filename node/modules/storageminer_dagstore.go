package modules

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"
	mdagstore "github.com/filecoin-project/lotus/markets/dagstore"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
)

const (
	EnvDAGStoreCopyConcurrency = "LOTUS_DAGSTORE_COPY_CONCURRENCY"
	DefaultDAGStoreDir         = "dagstore"
)

// NewMinerAPI creates a new MinerAPI adaptor for the dagstore mounts.
func NewMinerAPI(cfg config.DAGStoreConfig) func(fx.Lifecycle, repo.LockedRepo, dtypes.ProviderPieceStore, mdagstore.SectorAccessor) (mdagstore.MinerAPI, error) {
	return func(lc fx.Lifecycle, r repo.LockedRepo, pieceStore dtypes.ProviderPieceStore, sa mdagstore.SectorAccessor) (mdagstore.MinerAPI, error) {
		// caps the amount of concurrent calls to the storage, so that we don't
		// spam it during heavy processes like bulk migration.
		if v, ok := os.LookupEnv("LOTUS_DAGSTORE_MOUNT_CONCURRENCY"); ok {
			concurrency, err := strconv.Atoi(v)
			if err == nil {
				cfg.MaxConcurrencyStorageCalls = concurrency
			}
		}

		mountApi := mdagstore.NewMinerAPI(pieceStore, sa, cfg.MaxConcurrencyStorageCalls, cfg.MaxConcurrentUnseals)
		ready := make(chan error, 1)
		pieceStore.OnReady(func(err error) {
			ready <- err
		})
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				if err := <-ready; err != nil {
					return fmt.Errorf("aborting dagstore start; piecestore failed to start: %s", err)
				}
				return mountApi.Start(ctx)
			},
			OnStop: func(context.Context) error {
				return nil
			},
		})

		return mountApi, nil
	}
}

// DAGStore constructs a DAG store using the supplied minerAPI, and the
// user configuration. It returns both the DAGStore and the Wrapper suitable for
// passing to markets.
func DAGStore(cfg config.DAGStoreConfig) func(fx.Lifecycle, repo.LockedRepo, mdagstore.MinerAPI) (*dagstore.DAGStore, *mdagstore.Wrapper, error) {
	return func(lc fx.Lifecycle, r repo.LockedRepo, minerAPI mdagstore.MinerAPI) (*dagstore.DAGStore, *mdagstore.Wrapper, error) {
		// fall back to default root directory if not explicitly set in the config.
		if cfg.RootDir == "" {
			cfg.RootDir = filepath.Join(r.Path(), DefaultDAGStoreDir)
		}

		v, ok := os.LookupEnv(EnvDAGStoreCopyConcurrency)
		if ok {
			concurrency, err := strconv.Atoi(v)
			if err == nil {
				cfg.MaxConcurrentReadyFetches = concurrency
			}
		}

		dagst, w, err := mdagstore.NewDAGStore(cfg, minerAPI)
		if err != nil {
			return nil, nil, xerrors.Errorf("failed to create DAG store: %w", err)
		}

		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				return w.Start(ctx)
			},
			OnStop: func(context.Context) error {
				return w.Close()
			},
		})

		return dagst, w, nil
	}
}
