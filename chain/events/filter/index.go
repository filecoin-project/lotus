package filter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var pragmas = []string{
	"PRAGMA synchronous = normal",
	"PRAGMA temp_store = memory",
	"PRAGMA mmap_size = 30000000000",
	"PRAGMA page_size = 32768",
	"PRAGMA auto_vacuum = NONE",
	"PRAGMA automatic_index = OFF",
	"PRAGMA journal_mode = WAL",
	"PRAGMA wal_autocheckpoint = 256", // checkpoint @ 256 pages
	"PRAGMA journal_size_limit = 0",   // always reset journal and wal files
}

// Any changes to this schema should be matched for the `lotus-shed indexes backfill-events` command

var ddls = []string{
	`CREATE TABLE IF NOT EXISTS event (
		id INTEGER PRIMARY KEY,
		height INTEGER NOT NULL,
		tipset_key BLOB NOT NULL,
		tipset_key_cid BLOB NOT NULL,
		emitter_addr BLOB NOT NULL,
		event_index INTEGER NOT NULL,
		message_cid BLOB NOT NULL,
		message_index INTEGER NOT NULL,
		reverted INTEGER NOT NULL
	)`,

	createIndexEventEmitterAddr,
	createIndexEventTipsetKeyCid,
	createIndexEventHeight,
	createIndexEventReverted,

	`CREATE TABLE IF NOT EXISTS event_entry (
		event_id INTEGER,
		indexed INTEGER NOT NULL,
		flags BLOB NOT NULL,
		key TEXT NOT NULL,
		codec INTEGER,
		value BLOB NOT NULL
	)`,

	createTableEventsSeen,

	createIndexEventEntryIndexedKey,
	createIndexEventEntryCodecValue,
	createIndexEventEntryEventId,
	createIndexEventsSeenHeight,
	createIndexEventsSeenTipsetKeyCid,

	// metadata containing version of schema
	`CREATE TABLE IF NOT EXISTS _meta (
		version UINT64 NOT NULL UNIQUE
	)`,

	`INSERT OR IGNORE INTO _meta (version) VALUES (1)`,
	`INSERT OR IGNORE INTO _meta (version) VALUES (2)`,
	`INSERT OR IGNORE INTO _meta (version) VALUES (3)`,
	`INSERT OR IGNORE INTO _meta (version) VALUES (4)`,
	`INSERT OR IGNORE INTO _meta (version) VALUES (5)`,
	`INSERT OR IGNORE INTO _meta (version) VALUES (6)`,
}

var (
	log = logging.Logger("filter")
)

const (
	schemaVersion = 6

	eventExists          = `SELECT MAX(id) FROM event WHERE height=? AND tipset_key=? AND tipset_key_cid=? AND emitter_addr=? AND event_index=? AND message_cid=? AND message_index=?`
	insertEvent          = `INSERT OR IGNORE INTO event(height, tipset_key, tipset_key_cid, emitter_addr, event_index, message_cid, message_index, reverted) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`
	insertEntry          = `INSERT OR IGNORE INTO event_entry(event_id, indexed, flags, key, codec, value) VALUES(?, ?, ?, ?, ?, ?)`
	revertEventsInTipset = `UPDATE event SET reverted=true WHERE height=? AND tipset_key=?`
	restoreEvent         = `UPDATE event SET reverted=false WHERE height=? AND tipset_key=? AND tipset_key_cid=? AND emitter_addr=? AND event_index=? AND message_cid=? AND message_index=?`
	revertEventSeen      = `UPDATE events_seen SET reverted=true WHERE height=? AND tipset_key_cid=?`
	restoreEventSeen     = `UPDATE events_seen SET reverted=false WHERE height=? AND tipset_key_cid=?`
	upsertEventsSeen     = `INSERT INTO events_seen(height, tipset_key_cid, reverted) VALUES(?, ?, false) ON CONFLICT(height, tipset_key_cid) DO UPDATE SET reverted=false`
	isTipsetProcessed    = `SELECT COUNT(*) > 0 FROM events_seen WHERE tipset_key_cid=?`
	getMaxHeightInIndex  = `SELECT MAX(height) FROM events_seen`
	isHeightProcessed    = `SELECT COUNT(*) > 0 FROM events_seen WHERE height=?`

	createIndexEventEmitterAddr  = `CREATE INDEX IF NOT EXISTS event_emitter_addr ON event (emitter_addr)`
	createIndexEventTipsetKeyCid = `CREATE INDEX IF NOT EXISTS event_tipset_key_cid ON event (tipset_key_cid);`
	createIndexEventHeight       = `CREATE INDEX IF NOT EXISTS event_height ON event (height);`
	createIndexEventReverted     = `CREATE INDEX IF NOT EXISTS event_reverted ON event (reverted);`

	createIndexEventEntryIndexedKey = `CREATE INDEX IF NOT EXISTS event_entry_indexed_key ON event_entry (indexed, key);`
	createIndexEventEntryCodecValue = `CREATE INDEX IF NOT EXISTS event_entry_codec_value ON event_entry (codec, value);`
	createIndexEventEntryEventId    = `CREATE INDEX IF NOT EXISTS event_entry_event_id ON event_entry(event_id);`

	createTableEventsSeen = `CREATE TABLE IF NOT EXISTS events_seen (
		id INTEGER PRIMARY KEY,
		height INTEGER NOT NULL,
		tipset_key_cid BLOB NOT NULL,
		reverted INTEGER NOT NULL,
	    UNIQUE(height, tipset_key_cid)
	)`

	createIndexEventsSeenHeight       = `CREATE INDEX IF NOT EXISTS events_seen_height ON events_seen (height);`
	createIndexEventsSeenTipsetKeyCid = `CREATE INDEX IF NOT EXISTS events_seen_tipset_key_cid ON events_seen (tipset_key_cid);`
)

type EventIndex struct {
	db *sql.DB

	stmtEventExists          *sql.Stmt
	stmtInsertEvent          *sql.Stmt
	stmtInsertEntry          *sql.Stmt
	stmtRevertEventsInTipset *sql.Stmt
	stmtRestoreEvent         *sql.Stmt
	stmtUpsertEventsSeen     *sql.Stmt
	stmtRevertEventSeen      *sql.Stmt
	stmtRestoreEventSeen     *sql.Stmt

	stmtIsTipsetProcessed   *sql.Stmt
	stmtGetMaxHeightInIndex *sql.Stmt
	stmtIsHeightProcessed   *sql.Stmt

	mu           sync.Mutex
	subIdCounter uint64
	updateSubs   map[uint64]*updateSub
}

type updateSub struct {
	ctx    context.Context
	ch     chan EventIndexUpdated
	cancel context.CancelFunc
}

type EventIndexUpdated struct{}

func (ei *EventIndex) initStatements() (err error) {
	ei.stmtEventExists, err = ei.db.Prepare(eventExists)
	if err != nil {
		return xerrors.Errorf("prepare stmtEventExists: %w", err)
	}

	ei.stmtInsertEvent, err = ei.db.Prepare(insertEvent)
	if err != nil {
		return xerrors.Errorf("prepare stmtInsertEvent: %w", err)
	}

	ei.stmtInsertEntry, err = ei.db.Prepare(insertEntry)
	if err != nil {
		return xerrors.Errorf("prepare stmtInsertEntry: %w", err)
	}

	ei.stmtRevertEventsInTipset, err = ei.db.Prepare(revertEventsInTipset)
	if err != nil {
		return xerrors.Errorf("prepare stmtRevertEventsInTipset: %w", err)
	}

	ei.stmtRestoreEvent, err = ei.db.Prepare(restoreEvent)
	if err != nil {
		return xerrors.Errorf("prepare stmtRestoreEvent: %w", err)
	}

	ei.stmtUpsertEventsSeen, err = ei.db.Prepare(upsertEventsSeen)
	if err != nil {
		return xerrors.Errorf("prepare stmtUpsertEventsSeen: %w", err)
	}

	ei.stmtRevertEventSeen, err = ei.db.Prepare(revertEventSeen)
	if err != nil {
		return xerrors.Errorf("prepare stmtRevertEventSeen: %w", err)
	}

	ei.stmtRestoreEventSeen, err = ei.db.Prepare(restoreEventSeen)
	if err != nil {
		return xerrors.Errorf("prepare stmtRestoreEventSeen: %w", err)
	}

	ei.stmtIsTipsetProcessed, err = ei.db.Prepare(isTipsetProcessed)
	if err != nil {
		return xerrors.Errorf("prepare isTipsetProcessed: %w", err)
	}

	ei.stmtGetMaxHeightInIndex, err = ei.db.Prepare(getMaxHeightInIndex)
	if err != nil {
		return xerrors.Errorf("prepare getMaxHeightInIndex: %w", err)
	}

	ei.stmtIsHeightProcessed, err = ei.db.Prepare(isHeightProcessed)
	if err != nil {
		return xerrors.Errorf("prepare isHeightProcessed: %w", err)
	}

	return nil
}

func (ei *EventIndex) migrateToVersion2(ctx context.Context, chainStore *store.ChainStore) error {
	now := time.Now()

	tx, err := ei.db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	// rollback the transaction (a no-op if the transaction was already committed)
	defer func() { _ = tx.Rollback() }()

	// create some temporary indices to help speed up the migration
	_, err = tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS tmp_height_tipset_key_cid ON event (height,tipset_key_cid)")
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
	err = ei.db.QueryRowContext(ctx, "SELECT MIN(height) FROM event").Scan(&minHeight)
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

	// increment the schema version to 2 in _meta table.
	_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO _meta (version) VALUES (2)")
	if err != nil {
		return xerrors.Errorf("increment _meta version: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}

	log.Infof("Successfully migrated event index from version 1 to version 2 in %s", time.Since(now))

	return nil
}

// migrateToVersion3 migrates the schema from version 2 to version 3 by creating two indices:
// 1) an index on the event.emitter_addr column, and 2) an index on the event_entry.key column.
func (ei *EventIndex) migrateToVersion3(ctx context.Context) error {
	now := time.Now()

	tx, err := ei.db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// create index on event.emitter_addr.
	_, err = tx.ExecContext(ctx, createIndexEventEmitterAddr)
	if err != nil {
		return xerrors.Errorf("create index event_emitter_addr: %w", err)
	}

	// original v3 migration introduced an index:
	//	CREATE INDEX IF NOT EXISTS event_entry_key_index ON event_entry (key)
	// which has subsequently been removed in v4, so it's omitted here

	// increment the schema version to 3 in _meta table.
	_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO _meta (version) VALUES (3)")
	if err != nil {
		return xerrors.Errorf("increment _meta version: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}
	log.Infof("Successfully migrated event index from version 2 to version 3 in %s", time.Since(now))
	return nil
}

// migrateToVersion4 migrates the schema from version 3 to version 4 by adjusting indexes to match
// the query patterns of the event filter.
//
// First it drops indexes introduced in previous migrations:
//  1. the index on the event.height and event.tipset_key columns
//  2. the index on the event_entry.key column
//
// And then creating the following indices:
//  1. an index on the event.tipset_key_cid column
//  2. an index on the event.height column
//  3. an index on the event.reverted column
//  4. an index on the event_entry.indexed and event_entry.key columns
//  5. an index on the event_entry.codec and event_entry.value columns
//  6. an index on the event_entry.event_id column
func (ei *EventIndex) migrateToVersion4(ctx context.Context) error {
	now := time.Now()

	tx, err := ei.db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, create := range []struct {
		desc  string
		query string
	}{
		{"drop index height_tipset_key", "DROP INDEX IF EXISTS height_tipset_key;"},
		{"drop index event_entry_key_index", "DROP INDEX IF EXISTS event_entry_key_index;"},
		{"create index event_tipset_key_cid", createIndexEventTipsetKeyCid},
		{"create index event_height", createIndexEventHeight},
		{"create index event_reverted", createIndexEventReverted},
		{"create index event_entry_indexed_key", createIndexEventEntryIndexedKey},
		{"create index event_entry_codec_value", createIndexEventEntryCodecValue},
		{"create index event_entry_event_id", createIndexEventEntryEventId},
	} {
		_, err = tx.ExecContext(ctx, create.query)
		if err != nil {
			return xerrors.Errorf("%s: %w", create.desc, err)
		}
	}

	if _, err = tx.Exec("INSERT OR IGNORE INTO _meta (version) VALUES (4)"); err != nil {
		return xerrors.Errorf("increment _meta version: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}

	log.Infof("Successfully migrated event index from version 3 to version 4 in %s", time.Since(now))
	return nil
}

func (ei *EventIndex) migrateToVersion5(ctx context.Context) error {
	now := time.Now()

	tx, err := ei.db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmtEventIndexUpdate, err := tx.PrepareContext(ctx, "UPDATE event SET event_index = (SELECT COUNT(*) FROM event e2 WHERE e2.tipset_key_cid = event.tipset_key_cid AND e2.id <= event.id) - 1")
	if err != nil {
		return xerrors.Errorf("prepare stmtEventIndexUpdate: %w", err)
	}

	_, err = stmtEventIndexUpdate.ExecContext(ctx)
	if err != nil {
		return xerrors.Errorf("update event index: %w", err)
	}

	_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO _meta (version) VALUES (5)")
	if err != nil {
		return xerrors.Errorf("increment _meta version: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}

	log.Infof("Successfully migrated event index from version 4 to version 5 in %s", time.Since(now))
	return nil
}

func (ei *EventIndex) migrateToVersion6(ctx context.Context) error {
	now := time.Now()

	tx, err := ei.db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

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

	_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO _meta (version) VALUES (6)")
	if err != nil {
		return xerrors.Errorf("increment _meta version: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}

	ei.vacuumDBAndCheckpointWAL(ctx)

	log.Infof("Successfully migrated event index from version 5 to version 6 in %s", time.Since(now))
	return nil
}

func (ei *EventIndex) vacuumDBAndCheckpointWAL(ctx context.Context) {
	// During the large migrations, we have likely increased the WAL size a lot, so lets do some
	// simple DB administration to free up space (VACUUM followed by truncating the WAL file)
	// as this would be a good time to do it when no other writes are happening.
	log.Infof("Performing DB vacuum and wal checkpointing to free up space after the migration")
	_, err := ei.db.ExecContext(ctx, "VACUUM")
	if err != nil {
		log.Warnf("error vacuuming database: %s", err)
	}
	_, err = ei.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		log.Warnf("error checkpointing wal: %s", err)
	}
}

func NewEventIndex(ctx context.Context, path string, chainStore *store.ChainStore) (*EventIndex, error) {
	db, err := sql.Open("sqlite3", path+"?mode=rwc")
	if err != nil {
		return nil, xerrors.Errorf("open sqlite3 database: %w", err)
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, xerrors.Errorf("exec pragma %q: %w", pragma, err)
		}
	}

	eventIndex := EventIndex{db: db}

	q, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='_meta';")
	if q != nil {
		defer func() { _ = q.Close() }()
	}
	if errors.Is(err, sql.ErrNoRows) || !q.Next() {
		// empty database, create the schema
		for _, ddl := range ddls {
			if _, err := db.Exec(ddl); err != nil {
				_ = db.Close()
				return nil, xerrors.Errorf("exec ddl %q: %w", ddl, err)
			}
		}
	} else if err != nil {
		_ = db.Close()
		return nil, xerrors.Errorf("looking for _meta table: %w", err)
	} else {
		// check the schema version to see if we need to upgrade the database schema
		var version int
		err := db.QueryRow("SELECT max(version) FROM _meta").Scan(&version)
		if err != nil {
			_ = db.Close()
			return nil, xerrors.Errorf("invalid database version: no version found")
		}

		if version == 1 {
			log.Infof("Upgrading event index from version 1 to version 2")
			err = eventIndex.migrateToVersion2(ctx, chainStore)
			if err != nil {
				_ = db.Close()
				return nil, xerrors.Errorf("could not migrate event index schema from version 1 to version 2: %w", err)
			}
			version = 2
		}

		if version == 2 {
			log.Infof("Upgrading event index from version 2 to version 3")
			err = eventIndex.migrateToVersion3(ctx)
			if err != nil {
				_ = db.Close()
				return nil, xerrors.Errorf("could not migrate event index schema from version 2 to version 3: %w", err)
			}
			version = 3
		}

		if version == 3 {
			log.Infof("Upgrading event index from version 3 to version 4")
			err = eventIndex.migrateToVersion4(ctx)
			if err != nil {
				_ = db.Close()
				return nil, xerrors.Errorf("could not migrate event index schema from version 3 to version 4: %w", err)
			}
			version = 4
		}

		if version == 4 {
			log.Infof("Upgrading event index from version 4 to version 5")
			err = eventIndex.migrateToVersion5(ctx)
			if err != nil {
				_ = db.Close()
				return nil, xerrors.Errorf("could not migrate event index schema from version 4 to version 5: %w", err)
			}
			version = 5
		}

		if version == 5 {
			log.Infof("Upgrading event index from version 5 to version 6")
			err = eventIndex.migrateToVersion6(ctx)
			if err != nil {
				_ = db.Close()
				return nil, xerrors.Errorf("could not migrate event index schema from version 5 to version 6: %w", err)
			}
			version = 6
		}

		if version != schemaVersion {
			_ = db.Close()
			return nil, xerrors.Errorf("invalid database version: got %d, expected %d", version, schemaVersion)
		}
	}

	err = eventIndex.initStatements()
	if err != nil {
		_ = db.Close()
		return nil, xerrors.Errorf("error preparing eventIndex database statements: %w", err)
	}

	eventIndex.updateSubs = make(map[uint64]*updateSub)

	return &eventIndex, nil
}

func (ei *EventIndex) Close() error {
	if ei.db == nil {
		return nil
	}
	return ei.db.Close()
}

func (ei *EventIndex) SubscribeUpdates() (chan EventIndexUpdated, func()) {
	subCtx, subCancel := context.WithCancel(context.Background())
	ch := make(chan EventIndexUpdated)

	tSub := &updateSub{
		ctx:    subCtx,
		cancel: subCancel,
		ch:     ch,
	}

	ei.mu.Lock()
	subId := ei.subIdCounter
	ei.subIdCounter++
	ei.updateSubs[subId] = tSub
	ei.mu.Unlock()

	unSubscribeF := func() {
		ei.mu.Lock()
		tSub, ok := ei.updateSubs[subId]
		if !ok {
			ei.mu.Unlock()
			return
		}
		delete(ei.updateSubs, subId)
		ei.mu.Unlock()

		// cancel the subscription
		tSub.cancel()
	}

	return tSub.ch, unSubscribeF
}

func (ei *EventIndex) GetMaxHeightInIndex(ctx context.Context) (uint64, error) {
	row := ei.stmtGetMaxHeightInIndex.QueryRowContext(ctx)
	var maxHeight uint64
	err := row.Scan(&maxHeight)
	return maxHeight, err
}

func (ei *EventIndex) IsHeightProcessed(ctx context.Context, height uint64) (bool, error) {
	row := ei.stmtIsHeightProcessed.QueryRowContext(ctx, height)
	var exists bool
	err := row.Scan(&exists)
	return exists, err
}

func (ei *EventIndex) IsTipsetProcessed(ctx context.Context, tipsetKeyCid []byte) (bool, error) {
	row := ei.stmtIsTipsetProcessed.QueryRowContext(ctx, tipsetKeyCid)
	var exists bool
	err := row.Scan(&exists)
	return exists, err
}

func (ei *EventIndex) CollectEvents(ctx context.Context, te *TipSetEvents, revert bool, resolver func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool)) error {
	tx, err := ei.db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	// rollback the transaction (a no-op if the transaction was already committed)
	defer func() { _ = tx.Rollback() }()

	tsKeyCid, err := te.msgTs.Key().Cid()
	if err != nil {
		return xerrors.Errorf("tipset key cid: %w", err)
	}

	// lets handle the revert case first, since its simpler and we can simply mark all events in this tipset as reverted and return
	if revert {
		_, err = tx.Stmt(ei.stmtRevertEventsInTipset).Exec(te.msgTs.Height(), te.msgTs.Key().Bytes())
		if err != nil {
			return xerrors.Errorf("revert event: %w", err)
		}

		_, err = tx.Stmt(ei.stmtRevertEventSeen).Exec(te.msgTs.Height(), tsKeyCid.Bytes())
		if err != nil {
			return xerrors.Errorf("revert event seen: %w", err)
		}

		err = tx.Commit()
		if err != nil {
			return xerrors.Errorf("commit transaction: %w", err)
		}

		ei.mu.Lock()
		tSubs := make([]*updateSub, 0, len(ei.updateSubs))
		for _, tSub := range ei.updateSubs {
			tSubs = append(tSubs, tSub)
		}
		ei.mu.Unlock()

		for _, tSub := range tSubs {
			tSub := tSub
			select {
			case tSub.ch <- EventIndexUpdated{}:
			case <-tSub.ctx.Done():
				// subscription was cancelled, ignore
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	}

	// cache of lookups between actor id and f4 address
	addressLookups := make(map[abi.ActorID]address.Address)

	ems, err := te.messages(ctx)
	if err != nil {
		return xerrors.Errorf("load executed messages: %w", err)
	}

	eventCount := 0
	// iterate over all executed messages in this tipset and insert them into the database if they
	// don't exist, otherwise mark them as not reverted
	for msgIdx, em := range ems {
		for _, ev := range em.Events() {
			addr, found := addressLookups[ev.Emitter]
			if !found {
				var ok bool
				addr, ok = resolver(ctx, ev.Emitter, te.rctTs)
				if !ok {
					// not an address we will be able to match against
					continue
				}
				addressLookups[ev.Emitter] = addr
			}

			// check if this event already exists in the database
			var entryID sql.NullInt64
			err = tx.Stmt(ei.stmtEventExists).QueryRow(
				te.msgTs.Height(),          // height
				te.msgTs.Key().Bytes(),     // tipset_key
				tsKeyCid.Bytes(),           // tipset_key_cid
				addr.Bytes(),               // emitter_addr
				eventCount,                 // event_index
				em.Message().Cid().Bytes(), // message_cid
				msgIdx,                     // message_index
			).Scan(&entryID)
			if err != nil {
				return xerrors.Errorf("error checking if event exists: %w", err)
			}

			if !entryID.Valid {
				// event does not exist, lets insert it
				res, err := tx.Stmt(ei.stmtInsertEvent).Exec(
					te.msgTs.Height(),          // height
					te.msgTs.Key().Bytes(),     // tipset_key
					tsKeyCid.Bytes(),           // tipset_key_cid
					addr.Bytes(),               // emitter_addr
					eventCount,                 // event_index
					em.Message().Cid().Bytes(), // message_cid
					msgIdx,                     // message_index
					false,                      // reverted
				)
				if err != nil {
					return xerrors.Errorf("exec insert event: %w", err)
				}

				entryID.Int64, err = res.LastInsertId()
				if err != nil {
					return xerrors.Errorf("get last row id: %w", err)
				}

				// insert all the entries for this event
				for _, entry := range ev.Entries {
					_, err = tx.Stmt(ei.stmtInsertEntry).Exec(
						entryID.Int64,               // event_id
						isIndexedValue(entry.Flags), // indexed
						[]byte{entry.Flags},         // flags
						entry.Key,                   // key
						entry.Codec,                 // codec
						entry.Value,                 // value
					)
					if err != nil {
						return xerrors.Errorf("exec insert entry: %w", err)
					}
				}
			} else {
				// event already exists, lets mark it as not reverted
				res, err := tx.Stmt(ei.stmtRestoreEvent).Exec(
					te.msgTs.Height(),          // height
					te.msgTs.Key().Bytes(),     // tipset_key
					tsKeyCid.Bytes(),           // tipset_key_cid
					addr.Bytes(),               // emitter_addr
					eventCount,                 // event_index
					em.Message().Cid().Bytes(), // message_cid
					msgIdx,                     // message_index
				)
				if err != nil {
					return xerrors.Errorf("exec restore event: %w", err)
				}

				rowsAffected, err := res.RowsAffected()
				if err != nil {
					return xerrors.Errorf("error getting rows affected: %s", err)
				}

				// this is a sanity check as we should only ever be updating one event
				if rowsAffected != 1 {
					log.Warnf("restored %d events but expected only one to exist", rowsAffected)
				}
			}
			eventCount++
		}
	}

	// this statement will mark the tipset as processed and will insert a new row if it doesn't exist
	// or update the reverted field to false if it does
	_, err = tx.Stmt(ei.stmtUpsertEventsSeen).Exec(
		te.msgTs.Height(),
		tsKeyCid.Bytes(),
	)
	if err != nil {
		return xerrors.Errorf("exec upsert events seen: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}

	ei.mu.Lock()
	tSubs := make([]*updateSub, 0, len(ei.updateSubs))
	for _, tSub := range ei.updateSubs {
		tSubs = append(tSubs, tSub)
	}
	ei.mu.Unlock()

	for _, tSub := range tSubs {
		tSub := tSub
		select {
		case tSub.ch <- EventIndexUpdated{}:
		case <-tSub.ctx.Done():
			// subscription was cancelled, ignore
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// PrefillFilter fills a filter's collection of events from the historic index
func (ei *EventIndex) prefillFilter(ctx context.Context, f *eventFilter, excludeReverted bool) error {
	clauses := []string{}
	values := []any{}
	joins := []string{}

	if f.tipsetCid != cid.Undef {
		clauses = append(clauses, "event.tipset_key_cid=?")
		values = append(values, f.tipsetCid.Bytes())
	} else {
		if f.minHeight >= 0 {
			clauses = append(clauses, "event.height>=?")
			values = append(values, f.minHeight)
		}
		if f.maxHeight >= 0 {
			clauses = append(clauses, "event.height<=?")
			values = append(values, f.maxHeight)
		}
	}

	if excludeReverted {
		clauses = append(clauses, "event.reverted=?")
		values = append(values, false)
	}

	if len(f.addresses) > 0 {
		subclauses := make([]string, 0, len(f.addresses))
		for _, addr := range f.addresses {
			subclauses = append(subclauses, "emitter_addr=?")
			values = append(values, addr.Bytes())
		}
		clauses = append(clauses, "("+strings.Join(subclauses, " OR ")+")")
	}

	if len(f.keysWithCodec) > 0 {
		join := 0
		for key, vals := range f.keysWithCodec {
			if len(vals) > 0 {
				join++
				joinAlias := fmt.Sprintf("ee%d", join)
				joins = append(joins, fmt.Sprintf("event_entry %s on event.id=%[1]s.event_id", joinAlias))
				clauses = append(clauses, fmt.Sprintf("%s.indexed=1 AND %[1]s.key=?", joinAlias))
				values = append(values, key)
				subclauses := make([]string, 0, len(vals))
				for _, val := range vals {
					subclauses = append(subclauses, fmt.Sprintf("(%s.value=? AND %[1]s.codec=?)", joinAlias))
					values = append(values, val.Value, val.Codec)
				}
				clauses = append(clauses, "("+strings.Join(subclauses, " OR ")+")")
			}
		}
	}

	s := `SELECT
			event.id,
			event.height,
			event.tipset_key,
			event.tipset_key_cid,
			event.emitter_addr,
			event.event_index,
			event.message_cid,
			event.message_index,
			event.reverted,
			event_entry.flags,
			event_entry.key,
			event_entry.codec,
			event_entry.value
		FROM event JOIN event_entry ON event.id=event_entry.event_id`

	if len(joins) > 0 {
		s = s + ", " + strings.Join(joins, ", ")
	}

	if len(clauses) > 0 {
		s = s + " WHERE " + strings.Join(clauses, " AND ")
	}

	// retain insertion order of event_entry rows with the implicit _rowid_ column
	s += " ORDER BY event.height DESC, event_entry._rowid_ ASC"

	stmt, err := ei.db.Prepare(s)
	if err != nil {
		return xerrors.Errorf("prepare prefill query: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	q, err := stmt.QueryContext(ctx, values...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return xerrors.Errorf("exec prefill query: %w", err)
	}
	defer func() { _ = q.Close() }()

	var ces []*CollectedEvent
	var currentID int64 = -1
	var ce *CollectedEvent

	for q.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var row struct {
			id           int64
			height       uint64
			tipsetKey    []byte
			tipsetKeyCid []byte
			emitterAddr  []byte
			eventIndex   int
			messageCid   []byte
			messageIndex int
			reverted     bool
			flags        []byte
			key          string
			codec        uint64
			value        []byte
		}

		if err := q.Scan(
			&row.id,
			&row.height,
			&row.tipsetKey,
			&row.tipsetKeyCid,
			&row.emitterAddr,
			&row.eventIndex,
			&row.messageCid,
			&row.messageIndex,
			&row.reverted,
			&row.flags,
			&row.key,
			&row.codec,
			&row.value,
		); err != nil {
			return xerrors.Errorf("read prefill row: %w", err)
		}

		if row.id != currentID {
			if ce != nil {
				ces = append(ces, ce)
				ce = nil
				// Unfortunately we can't easily incorporate the max results limit into the query due to the
				// unpredictable number of rows caused by joins
				// Break here to stop collecting rows
				if f.maxResults > 0 && len(ces) >= f.maxResults {
					break
				}
			}

			currentID = row.id
			ce = &CollectedEvent{
				EventIdx: row.eventIndex,
				Reverted: row.reverted,
				Height:   abi.ChainEpoch(row.height),
				MsgIdx:   row.messageIndex,
			}

			ce.EmitterAddr, err = address.NewFromBytes(row.emitterAddr)
			if err != nil {
				return xerrors.Errorf("parse emitter addr: %w", err)
			}

			ce.TipSetKey, err = types.TipSetKeyFromBytes(row.tipsetKey)
			if err != nil {
				return xerrors.Errorf("parse tipsetkey: %w", err)
			}

			ce.MsgCid, err = cid.Cast(row.messageCid)
			if err != nil {
				return xerrors.Errorf("parse message cid: %w", err)
			}
		}

		ce.Entries = append(ce.Entries, types.EventEntry{
			Flags: row.flags[0],
			Key:   row.key,
			Codec: row.codec,
			Value: row.value,
		})
	}

	if ce != nil {
		ces = append(ces, ce)
	}

	if len(ces) == 0 {
		return nil
	}

	// collected event list is in inverted order since we selected only the most recent events
	// sort it into height order
	sort.Slice(ces, func(i, j int) bool { return ces[i].Height < ces[j].Height })
	f.setCollectedEvents(ces)

	return nil
}
