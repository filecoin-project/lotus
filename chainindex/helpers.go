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
	// if a database already exists, we try to delete it and create a new one
	if _, err := os.Stat(path); err == nil {
		if err = os.Remove(path); err != nil {
			return xerrors.Errorf("chainindex already exists at %s and can't be deleted", path)
		}
	}

	si, err := NewSqliteIndexer(path, cs, 0)
	if err != nil {
		return xerrors.Errorf("failed to create sqlite indexer: %w", err)
	}
	defer func() {
		_ = si.Close()
	}()

	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		curTs := cs.GetHeaviestTipSet()
		startHeight := curTs.Height()

		for curTs != nil {
			parentTs, err := cs.GetTipSetFromKey(ctx, curTs.Parents())
			if err != nil {
				return xerrors.Errorf("error getting parent tipset: %w", err)
			}

			if err := si.indexTipset(ctx, tx, curTs, parentTs, false); err != nil {
				log.Infof("stopping import after %d tipsets", startHeight-curTs.Height())
				break
			}

			curTs = parentTs
		}

		return nil
	})
	if err != nil {
		return xerrors.Errorf("failed populate from snapshot: %w", err)
	}

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
				log.Errorw("error indexing signed Mpool message", "error", err)
			}
		}
	}
}

// revert function for observer
func toTipsetKeyCidBytes(ts *types.TipSet) ([]byte, error) {
	tsKeyCid, err := ts.Key().Cid()
	if err != nil {
		return nil, xerrors.Errorf("error getting tipset key cid: %w", err)
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
