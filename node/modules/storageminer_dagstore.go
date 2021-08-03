package modules

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/lotus/markets/dagstore"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/go-datastore"
	levelds "github.com/ipfs/go-ds-leveldb"
	measure "github.com/ipfs/go-ds-measure"
	ldbopts "github.com/syndtr/goleveldb/leveldb/opt"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

func NewLotusAccessor(lc fx.Lifecycle,
	pieceStore dtypes.ProviderPieceStore,
	rpn retrievalmarket.RetrievalProviderNode,
) (dagstore.MinerAPI, error) {
	mountApi := dagstore.NewMinerAPI(pieceStore, rpn)
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

func DAGStoreWrapper(
	lc fx.Lifecycle,
	r repo.LockedRepo,
	lotusAccessor dagstore.MinerAPI,
) (*dagstore.Wrapper, error) {
	dir := filepath.Join(r.Path(), dagStore)
	ds, err := newDAGStoreDatastore(dir)
	if err != nil {
		return nil, err
	}

	var maxCopies = 2
	// TODO replace env with config.toml attribute.
	v, ok := os.LookupEnv("LOTUS_DAGSTORE_COPY_CONCURRENCY")
	if ok {
		concurrency, err := strconv.Atoi(v)
		if err == nil {
			maxCopies = concurrency
		}
	}

	cfg := dagstore.MarketDAGStoreConfig{
		TransientsDir:             filepath.Join(dir, "transients"),
		IndexDir:                  filepath.Join(dir, "index"),
		Datastore:                 ds,
		GCInterval:                1 * time.Minute,
		MaxConcurrentIndex:        5,
		MaxConcurrentReadyFetches: maxCopies,
	}

	dsw, err := dagstore.NewWrapper(cfg, lotusAccessor)
	if err != nil {
		return nil, xerrors.Errorf("failed to create DAG store wrapper: %w", err)
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return dsw.Start(ctx)
		},
		OnStop: func(context.Context) error {
			return dsw.Close()
		},
	})
	return dsw, nil
}

// newDAGStoreDatastore creates a datastore under the given base directory
// for dagstore metadata.
func newDAGStoreDatastore(baseDir string) (datastore.Batching, error) {
	// Create a datastore directory under the base dir if it doesn't already exist
	dsDir := filepath.Join(baseDir, "datastore")
	if err := os.MkdirAll(dsDir, 0755); err != nil {
		return nil, xerrors.Errorf("failed to create directory %s for DAG store datastore: %w", dsDir, err)
	}

	// Create a new LevelDB datastore
	ds, err := levelds.NewDatastore(dsDir, &levelds.Options{
		Compression: ldbopts.NoCompression,
		NoSync:      false,
		Strict:      ldbopts.StrictAll,
		ReadOnly:    false,
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to open datastore for DAG store: %w", err)
	}
	// Keep statistics about the datastore
	mds := measure.New("measure.", ds)
	return mds, nil
}
