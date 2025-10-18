package index

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"time"

	ipld "github.com/ipfs/go-ipld-format"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
)

const maxRetries = 3
const retryDelay = 150 * time.Millisecond

// PopulateFromSnapshot initializes and populates the chain index from a snapshot.
//
// This function creates a new Index at the specified path and populates
// it by using the chain state from the provided ChainStore. It starts from the heaviest
// tipset and works backwards, indexing each tipset until it reaches the genesis
// block or encounters a tipset for which it is unable to find messages in the chain store.
//
// Important Notes:
//  1. This function assumes that the snapshot has already been imported into the ChainStore.
//  2. Events are not populated in the index because snapshots do not contain event data,
//     and messages are not re-executed during this process. The resulting index will
//     only contain tipsets and messages.
//  3. This function will delete any existing database at the specified path before
//     creating a new one.
func PopulateFromSnapshot(ctx context.Context, path string, cs ChainStore) error {
	log.Infof("populating chainindex at path %s from snapshot", path)
	// Check if a database already exists and attempt to delete it
	if _, err := os.Stat(path); err == nil {
		log.Infof("deleting existing chainindex at %s", path)
		if err = os.Remove(path); err != nil {
			return xerrors.Errorf("failed to delete existing chainindex at %s: %w", path, err)
		}
	}

	si, err := NewSqliteIndexer(path, cs, 0, false, 0)
	if err != nil {
		return xerrors.Errorf("failed to create sqlite indexer: %w", err)
	}
	defer func() {
		if closeErr := si.Close(); closeErr != nil {
			log.Errorf("failed to close sqlite indexer: %s", closeErr)
		}
	}()

	totalIndexed := 0

	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		head := cs.GetHeaviestTipSet()
		curTs := head
		log.Infof("starting to populate chainindex from snapshot at head height %d", head.Height())

		for curTs != nil {
			if err := si.indexTipset(ctx, tx, curTs); err != nil {
				if ipld.IsNotFound(err) {
					log.Infof("stopping chainindex population at height %d as snapshot only contains data upto this height; error is: %s", curTs.Height(), err)
					break
				}

				return xerrors.Errorf("failed to populate chainindex from snapshot at height %d: %w", curTs.Height(), err)
			}
			totalIndexed++

			curTs, err = cs.GetTipSetFromKey(ctx, curTs.Parents())
			if err != nil {
				return xerrors.Errorf("failed to get parent tipset: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return xerrors.Errorf("failed to populate chainindex from snapshot: %w", err)
	}

	log.Infof("Successfully populated chainindex from snapshot with %d tipsets", totalIndexed)
	return nil
}

func toTipsetKeyCidBytes(ts *types.TipSet) ([]byte, error) {
	if ts == nil {
		return nil, errors.New("failed to get tipset key cid: tipset is nil")
	}
	tsKeyCid, err := ts.Key().Cid()
	if err != nil {
		return nil, err
	}
	return tsKeyCid.Bytes(), nil
}

func withTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var tx *sql.Tx
		tx, err = db.BeginTx(ctx, nil)
		if err != nil {
			return xerrors.Errorf("failed to begin transaction: %w", err)
		}

		defer func() {
			if p := recover(); p != nil {
				// A panic occurred, rollback and repanic
				if tx != nil {
					_ = tx.Rollback()
				}
				panic(p)
			}
		}()

		err = fn(tx)
		if err == nil {
			if commitErr := tx.Commit(); commitErr != nil {
				return xerrors.Errorf("failed to commit transaction: %w", commitErr)
			}
			return nil
		}

		_ = tx.Rollback()

		if !isRetryableError(err) {
			return xerrors.Errorf("transaction failed: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
			// Retry after delay
		}
	}

	return xerrors.Errorf("transaction failed after %d retries; last error: %w", maxRetries, err)
}

func isRetryableError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "database is locked")
}

func isIndexedFlag(b uint8) bool {
	// currently we mark the full entry as indexed if either the key
	// or the value are indexed; in the future we will need finer-grained
	// management of indices
	return b&(types.EventFlagIndexedKey|types.EventFlagIndexedValue) > 0
}
