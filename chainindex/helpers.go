package chainindex

import (
	"context"
	"database/sql"
	"os"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

func PopulateFromSnapshot(ctx context.Context, path string, cs ChainStore) error {
	// Check if a database already exists and attempt to delete it
	if _, err := os.Stat(path); err == nil {
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
		startHeight := head.Height()
		curTs := head
		log.Infof("starting index hydration from snapshot at height %d", startHeight)

		for curTs != nil {
			if err := si.indexTipset(ctx, tx, curTs); err != nil {
				log.Infof("stopping import after %d tipsets with final error: %s", startHeight-curTs.Height(), err)
				break
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
		return xerrors.Errorf("failed to populate from snapshot: %w", err)
	}

	log.Infof("Successfully populated chainindex from snapshot with %d tipsets", totalIndexed)
	return nil
}

func WaitForMpoolUpdates(ctx context.Context, ch <-chan api.MpoolUpdate, indexer Indexer) {
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case u := <-ch:
			if u.Type != api.MpoolAdd {
				continue
			}
			if u.Message == nil {
				continue
			}
			err := indexer.IndexSignedMessage(ctx, u.Message)
			if err != nil {
				log.Errorw("failed to index signed Mpool message", "error", err)
			}
		}
	}
}

// revert function for observer
func toTipsetKeyCidBytes(ts *types.TipSet) ([]byte, error) {
	tsKeyCid, err := ts.Key().Cid()
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset key cid: %w", err)
	}
	return tsKeyCid.Bytes(), nil
}

func withTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) (err error) {
	var tx *sql.Tx
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			// A panic occurred, rollback and repanic
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			// Something went wrong, rollback
			_ = tx.Rollback()
		} else {
			// All good, commit
			err = tx.Commit()
		}
	}()

	err = fn(tx)
	return
}

func isIndexedValue(b uint8) bool {
	// currently we mark the full entry as indexed if either the key
	// or the value are indexed; in the future we will need finer-grained
	// management of indices
	return b&(types.EventFlagIndexedKey|types.EventFlagIndexedValue) > 0
}
