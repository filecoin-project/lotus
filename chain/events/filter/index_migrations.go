package filter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/lib/sqlite"
)

func migrationVersion2(db *sql.DB, chainStore *store.ChainStore) sqlite.MigrationFunc {
	return func(ctx context.Context, tx *sql.Tx) error {
		// create some temporary indices to help speed up the migration
		_, err := tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS tmp_height_tipset_key_cid ON event (height,tipset_key_cid)")
		if err != nil {
			return xerrors.Errorf("create index tmp_height_tipset_key_cid: %w", err)
		}
		_, err = tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS tmp_tipset_key_cid ON event (tipset_key_cid)")
		if err != nil {
			return xerrors.Errorf("create index tmp_tipset_key_cid: %w", err)
		}

		stmtDeleteOffChainEvent, err := tx.PrepareContext(ctx, "DELETE FROM event WHERE tipset_key_cid!=? and height=?")
		if err != nil {
			return xerrors.Errorf("prepare stmtDeleteOffChainEvent: %w", err)
		}

		stmtSelectEvent, err := tx.PrepareContext(ctx, "SELECT id FROM event WHERE tipset_key_cid=? ORDER BY message_index ASC, event_index ASC, id DESC LIMIT 1")
		if err != nil {
			return xerrors.Errorf("prepare stmtSelectEvent: %w", err)
		}

		stmtDeleteEvent, err := tx.PrepareContext(ctx, "DELETE FROM event WHERE tipset_key_cid=? AND id<?")
		if err != nil {
			return xerrors.Errorf("prepare stmtDeleteEvent: %w", err)
		}

		// get the lowest height tipset
		var minHeight sql.NullInt64
		err = db.QueryRowContext(ctx, "SELECT MIN(height) FROM event").Scan(&minHeight)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}

			return xerrors.Errorf("query min height: %w", err)
		}
		log.Infof("Migrating events from head to %d", minHeight.Int64)

		currTs := chainStore.GetHeaviestTipSet()

		for int64(currTs.Height()) >= minHeight.Int64 {
			if currTs.Height()%1000 == 0 {
				log.Infof("Migrating height %d (remaining %d)", currTs.Height(), int64(currTs.Height())-minHeight.Int64)
			}

			tsKey := currTs.Parents()
			currTs, err = chainStore.GetTipSetFromKey(ctx, tsKey)
			if err != nil {
				return xerrors.Errorf("get tipset from key: %w", err)
			}
			log.Debugf("Migrating height %d", currTs.Height())

			tsKeyCid, err := currTs.Key().Cid()
			if err != nil {
				return fmt.Errorf("tipset key cid: %w", err)
			}

			// delete all events that are not in the canonical chain
			_, err = stmtDeleteOffChainEvent.Exec(tsKeyCid.Bytes(), currTs.Height())
			if err != nil {
				return xerrors.Errorf("delete off chain event: %w", err)
			}

			// find the first eventId from the last time the tipset was applied
			var eventId sql.NullInt64
			err = stmtSelectEvent.QueryRow(tsKeyCid.Bytes()).Scan(&eventId)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				return xerrors.Errorf("select event: %w", err)
			}

			// this tipset might not have any events which is ok
			if !eventId.Valid {
				continue
			}
			log.Debugf("Deleting all events with id < %d at height %d", eventId.Int64, currTs.Height())

			res, err := stmtDeleteEvent.Exec(tsKeyCid.Bytes(), eventId.Int64)
			if err != nil {
				return xerrors.Errorf("delete event: %w", err)
			}

			nrRowsAffected, err := res.RowsAffected()
			if err != nil {
				return xerrors.Errorf("rows affected: %w", err)
			}
			log.Debugf("deleted %d events from tipset %s", nrRowsAffected, tsKeyCid.String())
		}

		// delete all entries that have an event_id that doesn't exist (since we don't have a foreign
		// key constraint that gives us cascading deletes)
		res, err := tx.ExecContext(ctx, "DELETE FROM event_entry WHERE event_id NOT IN (SELECT id FROM event)")
		if err != nil {
			return xerrors.Errorf("delete event_entry: %w", err)
		}

		nrRowsAffected, err := res.RowsAffected()
		if err != nil {
			return xerrors.Errorf("rows affected: %w", err)
		}
		log.Infof("Cleaned up %d entries that had deleted events\n", nrRowsAffected)

		// drop the temporary indices after the migration
		_, err = tx.ExecContext(ctx, "DROP INDEX IF EXISTS tmp_tipset_key_cid")
		if err != nil {
			return xerrors.Errorf("drop index tmp_tipset_key_cid: %w", err)
		}
		_, err = tx.ExecContext(ctx, "DROP INDEX IF EXISTS tmp_height_tipset_key_cid")
		if err != nil {
			return xerrors.Errorf("drop index tmp_height_tipset_key_cid: %w", err)
		}

		// original v2 migration introduced an index:
		//	CREATE INDEX IF NOT EXISTS height_tipset_key ON event (height,tipset_key)
		// which has subsequently been removed in v4, so it's omitted here

		return nil
	}
}

// migrationVersion3 migrates the schema from version 2 to version 3 by creating two indices:
// 1) an index on the event.emitter_addr column, and 2) an index on the event_entry.key column.
//
// As of version 7, these indices have been removed as they were found to be a performance
// hindrance. This migration is now a no-op.
func migrationVersion3(ctx context.Context, tx *sql.Tx) error {
	return nil
}

// migrationVersion4 migrates the schema from version 3 to version 4 by adjusting indexes to match
// the query patterns of the event filter.
//
// First it drops indexes introduced in previous migrations:
//  1. the index on the event.height and event.tipset_key columns
//  2. the index on the event_entry.key column
//
// And then creating the following indices:
//  1. an index on the event.tipset_key_cid column
//  2. an index on the event.height column
//  3. an index on the event.reverted column (removed in version 7)
//  4. a composite index on the event_entry.indexed and event_entry.key columns (removed in version 7)
//  5. a composite index on the event_entry.codec and event_entry.value columns (removed in version 7)
//  6. an index on the event_entry.event_id column
//
// Indexes 3, 4, and 5 were removed in version 7 as they were found to be a performance hindrance so
// are omitted here.
func migrationVersion4(ctx context.Context, tx *sql.Tx) error {
	for _, create := range []struct {
		desc  string
		query string
	}{
		{"drop index height_tipset_key", "DROP INDEX IF EXISTS height_tipset_key;"},
		{"drop index event_entry_key_index", "DROP INDEX IF EXISTS event_entry_key_index;"},
		{"create index event_tipset_key_cid", createIndexEventTipsetKeyCid},
		{"create index event_height", createIndexEventHeight},
		{"create index event_entry_event_id", createIndexEventEntryEventId},
	} {
		if _, err := tx.ExecContext(ctx, create.query); err != nil {
			return xerrors.Errorf("%s: %w", create.desc, err)
		}
	}

	return nil
}

// migrationVersion5 migrates the schema from version 4 to version 5 by updating the event_index
// to be 0-indexed within a tipset.
func migrationVersion5(ctx context.Context, tx *sql.Tx) error {
	stmtEventIndexUpdate, err := tx.PrepareContext(ctx, "UPDATE event SET event_index = (SELECT COUNT(*) FROM event e2 WHERE e2.tipset_key_cid = event.tipset_key_cid AND e2.id <= event.id) - 1")
	if err != nil {
		return xerrors.Errorf("prepare stmtEventIndexUpdate: %w", err)
	}

	_, err = stmtEventIndexUpdate.ExecContext(ctx)
	if err != nil {
		return xerrors.Errorf("update event index: %w", err)
	}

	return nil
}

// migrationVersion6 migrates the schema from version 5 to version 6 by creating a new table
// events_seen that tracks the tipsets that have been seen by the event filter and populating it
// with the tipsets that have events in the event table.
func migrationVersion6(ctx context.Context, tx *sql.Tx) error {
	stmtCreateTableEventsSeen, err := tx.PrepareContext(ctx, createTableEventsSeen)
	if err != nil {
		return xerrors.Errorf("prepare stmtCreateTableEventsSeen: %w", err)
	}
	_, err = stmtCreateTableEventsSeen.ExecContext(ctx)
	if err != nil {
		return xerrors.Errorf("create table events_seen: %w", err)
	}

	_, err = tx.ExecContext(ctx, createIndexEventsSeenHeight)
	if err != nil {
		return xerrors.Errorf("create index events_seen_height: %w", err)
	}
	_, err = tx.ExecContext(ctx, createIndexEventsSeenTipsetKeyCid)
	if err != nil {
		return xerrors.Errorf("create index events_seen_tipset_key_cid: %w", err)
	}

	// INSERT an entry in the events_seen table for all epochs we do have events for in our DB
	_, err = tx.ExecContext(ctx, `
    INSERT OR IGNORE INTO events_seen (height, tipset_key_cid, reverted)
    SELECT DISTINCT height, tipset_key_cid, reverted FROM event
`)
	if err != nil {
		return xerrors.Errorf("insert events into events_seen: %w", err)
	}

	return nil
}

// migrationVersion7 migrates the schema from version 6 to version 7 by dropping the following
// indices:
//  1. the index on the event.emitter_addr column
//  2. the index on the event.reverted column
//  3. the composite index on the event_entry.indexed and event_entry.key columns
//  4. the composite index on the event_entry.codec and event_entry.value columns
//
// These indices were found to be a performance hindrance as they prevent SQLite from using the
// intended initial indexes on height or tipset_key_cid in many query variations. Without additional
// indices to fall-back on, SQLite is forced to narrow down each query via height or tipset_key_cid
// which is the desired behavior.
func migrationVersion7(ctx context.Context, tx *sql.Tx) error {
	for _, drop := range []struct {
		desc  string
		query string
	}{
		{"drop index event_emitter_addr", "DROP INDEX IF EXISTS event_emitter_addr;"},
		{"drop index event_reverted", "DROP INDEX IF EXISTS event_reverted;"},
		{"drop index event_entry_indexed_key", "DROP INDEX IF EXISTS event_entry_indexed_key;"},
		{"drop index event_entry_codec_value", "DROP INDEX IF EXISTS event_entry_codec_value;"},
	} {
		if _, err := tx.ExecContext(ctx, drop.query); err != nil {
			return xerrors.Errorf("%s: %w", drop.desc, err)
		}
	}

	return nil
}
