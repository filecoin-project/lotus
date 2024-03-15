package filter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	"PRAGMA read_uncommitted = ON",
}

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

	createIndexEventHeightTipsetKey,
	createIndexEventEmitterAddr,

	`CREATE TABLE IF NOT EXISTS event_entry (
		event_id INTEGER,
		indexed INTEGER NOT NULL,
		flags BLOB NOT NULL,
		key TEXT NOT NULL,
		codec INTEGER,
		value BLOB NOT NULL
	)`,

	createIndexEventEntryKey,

	// metadata containing version of schema
	`CREATE TABLE IF NOT EXISTS _meta (
		version UINT64 NOT NULL UNIQUE
	)`,

	`INSERT OR IGNORE INTO _meta (version) VALUES (1)`,
	`INSERT OR IGNORE INTO _meta (version) VALUES (2)`,
	`INSERT OR IGNORE INTO _meta (version) VALUES (3)`,
}

var (
	log = logging.Logger("filter")
)

const (
	schemaVersion = 3

	eventExists          = `SELECT MAX(id) FROM event WHERE height=? AND tipset_key=? AND tipset_key_cid=? AND emitter_addr=? AND event_index=? AND message_cid=? AND message_index=?`
	insertEvent          = `INSERT OR IGNORE INTO event(height, tipset_key, tipset_key_cid, emitter_addr, event_index, message_cid, message_index, reverted) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`
	insertEntry          = `INSERT OR IGNORE INTO event_entry(event_id, indexed, flags, key, codec, value) VALUES(?, ?, ?, ?, ?, ?)`
	revertEventsInTipset = `UPDATE event SET reverted=true WHERE height=? AND tipset_key=?`
	restoreEvent         = `UPDATE event SET reverted=false WHERE height=? AND tipset_key=? AND tipset_key_cid=? AND emitter_addr=? AND event_index=? AND message_cid=? AND message_index=?`

	createIndexEventHeightTipsetKey = `CREATE INDEX IF NOT EXISTS height_tipset_key ON event (height,tipset_key)`
	createIndexEventEmitterAddr     = `CREATE INDEX IF NOT EXISTS event_emitter_addr ON event (emitter_addr)`
	createIndexEventEntryKey        = `CREATE INDEX IF NOT EXISTS event_entry_key_index ON event_entry (key)`
)

type EventIndex struct {
	db *sql.DB

	stmtEventExists          *sql.Stmt
	stmtInsertEvent          *sql.Stmt
	stmtInsertEntry          *sql.Stmt
	stmtRevertEventsInTipset *sql.Stmt
	stmtRestoreEvent         *sql.Stmt
}

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
	log.Infof("cleaned up %d entries that had deleted events\n", nrRowsAffected)

	// drop the temporary indices after the migration
	_, err = tx.ExecContext(ctx, "DROP INDEX IF EXISTS tmp_tipset_key_cid")
	if err != nil {
		return xerrors.Errorf("drop index tmp_tipset_key_cid: %w", err)
	}
	_, err = tx.ExecContext(ctx, "DROP INDEX IF EXISTS tmp_height_tipset_key_cid")
	if err != nil {
		return xerrors.Errorf("drop index tmp_height_tipset_key_cid: %w", err)
	}

	// create the final index on event.height and event.tipset_key
	_, err = tx.ExecContext(ctx, createIndexEventHeightTipsetKey)
	if err != nil {
		return xerrors.Errorf("create index height_tipset_key: %w", err)
	}

	// increment the schema version to 2 in _meta table.
	_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO _meta (version) VALUES (2)")
	if err != nil {
		return xerrors.Errorf("increment _meta version: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}

	// during the migration, we have likely increased the WAL size a lot, so lets do some
	// simple DB administration to free up space (VACUUM followed by truncating the WAL file)
	// as this would be a good time to do it when no other writes are happening
	log.Infof("Performing DB vacuum and wal checkpointing to free up space after the migration")
	_, err = ei.db.ExecContext(ctx, "VACUUM")
	if err != nil {
		log.Warnf("error vacuuming database: %s", err)
	}
	_, err = ei.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		log.Warnf("error checkpointing wal: %s", err)
	}

	log.Infof("Successfully migrated events to version 2 in %s", time.Since(now))

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

	// create index on event_entry.key index.
	_, err = tx.ExecContext(ctx, createIndexEventEntryKey)
	if err != nil {
		return xerrors.Errorf("create index event_entry_key_index: %w", err)
	}

	// increment the schema version to 3 in _meta table.
	_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO _meta (version) VALUES (3)")
	if err != nil {
		return xerrors.Errorf("increment _meta version: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}
	log.Infof("Successfully migrated events to version 3 in %s", time.Since(now))
	return nil
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
			log.Infof("upgrading event index from version 1 to version 2")
			err = eventIndex.migrateToVersion2(ctx, chainStore)
			if err != nil {
				_ = db.Close()
				return nil, xerrors.Errorf("could not migrate sql data to version 2: %w", err)
			}
			version = 2
		}

		if version == 2 {
			log.Infof("upgrading event index from version 2 to version 3")
			err = eventIndex.migrateToVersion3(ctx)
			if err != nil {
				_ = db.Close()
				return nil, xerrors.Errorf("could not migrate sql data to version 2: %w", err)
			}
			version = 3
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

	return &eventIndex, nil
}

func (ei *EventIndex) Close() error {
	if ei.db == nil {
		return nil
	}
	return ei.db.Close()
}

func (ei *EventIndex) CollectEvents(ctx context.Context, te *TipSetEvents, revert bool, resolver func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool)) error {
	tx, err := ei.db.Begin()
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	// rollback the transaction (a no-op if the transaction was already committed)
	defer func() { _ = tx.Rollback() }()

	// lets handle the revert case first, since its simpler and we can simply mark all events events in this tipset as reverted and return
	if revert {
		_, err = tx.Stmt(ei.stmtRevertEventsInTipset).Exec(te.msgTs.Height(), te.msgTs.Key().Bytes())
		if err != nil {
			return xerrors.Errorf("revert event: %w", err)
		}

		err = tx.Commit()
		if err != nil {
			return xerrors.Errorf("commit transaction: %w", err)
		}

		return nil
	}

	// cache of lookups between actor id and f4 address
	addressLookups := make(map[abi.ActorID]address.Address)

	ems, err := te.messages(ctx)
	if err != nil {
		return xerrors.Errorf("load executed messages: %w", err)
	}

	// iterate over all executed messages in this tipset and insert them into the database if they
	// don't exist, otherwise mark them as not reverted
	for msgIdx, em := range ems {
		for evIdx, ev := range em.Events() {
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

			tsKeyCid, err := te.msgTs.Key().Cid()
			if err != nil {
				return xerrors.Errorf("tipset key cid: %w", err)
			}

			// check if this event already exists in the database
			var entryID sql.NullInt64
			err = tx.Stmt(ei.stmtEventExists).QueryRow(
				te.msgTs.Height(),          // height
				te.msgTs.Key().Bytes(),     // tipset_key
				tsKeyCid.Bytes(),           // tipset_key_cid
				addr.Bytes(),               // emitter_addr
				evIdx,                      // event_index
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
					evIdx,                      // event_index
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
					evIdx,                      // event_index
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
		}
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}

	return nil
}

// prefillFilter fills a filter's collection of events from the historic index
func (ei *EventIndex) prefillFilter(ctx context.Context, f *eventFilter, excludeReverted bool) error {
	var (
		clauses, joins []string
		values         []any
	)

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

	s += " ORDER BY event.height DESC"

	stmt, err := ei.db.Prepare(s)
	if err != nil {
		return xerrors.Errorf("prepare prefill query: %w", err)
	}

	q, err := stmt.QueryContext(ctx, values...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return xerrors.Errorf("exec prefill query: %w", err)
	}

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
