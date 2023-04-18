package filter

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	_ "github.com/mattn/go-sqlite3"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

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

	`CREATE TABLE IF NOT EXISTS event_entry (
		event_id INTEGER,
		indexed INTEGER NOT NULL,
		flags BLOB NOT NULL,
		key TEXT NOT NULL,
		codec INTEGER,
		value BLOB NOT NULL
	)`,

	// metadata containing version of schema
	`CREATE TABLE IF NOT EXISTS _meta (
    	version UINT64 NOT NULL UNIQUE
	)`,

	// set the version
	`INSERT OR IGNORE INTO _meta (version) VALUES (` + strconv.Itoa(schemaVersion) + `)`,
}

const schemaVersion = 2

const (
	insertEvent = `INSERT OR IGNORE INTO event
	(height, tipset_key, tipset_key_cid, emitter_addr, event_index, message_cid, message_index, reverted)
	VALUES(?, ?, ?, ?, ?, ?, ?, ?)`

	insertEntry = `INSERT OR IGNORE INTO event_entry
	(event_id, indexed, flags, key, codec, value)
	VALUES(?, ?, ?, ?, ?, ?)`
)

var log = logging.Logger("event_index")

type EventIndex struct {
	db *sql.DB
}

func NewEventIndex(path string) (*EventIndex, error) {
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

	var version int
	q, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name='_meta';")
	if err == sql.ErrNoRows || !q.Next() {
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
	}

	row := db.QueryRow("SELECT max(version) FROM _meta")
	if err := row.Scan(&version); err != nil {
		_ = db.Close()
		return nil, xerrors.Errorf("invalid database version: no version found")
	}

	// Create the event index.
	idx := &EventIndex{db: db}

	// Run any necessary migrations.
	if version != schemaVersion {
		if err := idx.RunMigration(version, schemaVersion); err != nil {
			return nil, xerrors.Errorf("failed to run event index migration from v%d to v%d: %w", version, schemaVersion, err)
		}
	}

	return idx, nil
}

func (ei *EventIndex) Close() error {
	if ei.db == nil {
		return nil
	}
	return ei.db.Close()
}

func (ei *EventIndex) CollectEvents(ctx context.Context, te *TipSetEvents, revert bool, resolver func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool)) error {
	// cache of lookups between actor id and f4 address

	addressLookups := make(map[abi.ActorID]address.Address)

	ems, err := te.messages(ctx)
	if err != nil {
		return xerrors.Errorf("load executed messages: %w", err)
	}

	tx, err := ei.db.Begin()
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	stmtEvent, err := tx.Prepare(insertEvent)
	if err != nil {
		return xerrors.Errorf("prepare insert event: %w", err)
	}
	stmtEntry, err := tx.Prepare(insertEntry)
	if err != nil {
		return xerrors.Errorf("prepare insert entry: %w", err)
	}

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

			res, err := stmtEvent.Exec(
				te.msgTs.Height(),          // height
				te.msgTs.Key().Bytes(),     // tipset_key
				tsKeyCid.Bytes(),           // tipset_key_cid
				addr.Bytes(),               // emitter_addr
				evIdx,                      // event_index
				em.Message().Cid().Bytes(), // message_cid
				msgIdx,                     // message_index
				revert,                     // reverted
			)
			if err != nil {
				return xerrors.Errorf("exec insert event: %w", err)
			}

			lastID, err := res.LastInsertId()
			if err != nil {
				return xerrors.Errorf("get last row id: %w", err)
			}

			for _, entry := range ev.Entries {
				_, err := stmtEntry.Exec(
					lastID,                      // event_id
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
		}
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}

	return nil
}

// PrefillFilter fills a filter's collection of events from the historic index
func (ei *EventIndex) PrefillFilter(ctx context.Context, f *EventFilter) error {
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

	if len(f.addresses) > 0 {
		subclauses := []string{}
		for _, addr := range f.addresses {
			subclauses = append(subclauses, "emitter_addr=?")
			values = append(values, addr.Bytes())
		}
		clauses = append(clauses, "("+strings.Join(subclauses, " OR ")+")")
	}

	if len(f.keys) > 0 {
		join := 0
		for key, vals := range f.keys {
			if len(vals) > 0 {
				join++
				joinAlias := fmt.Sprintf("ee%d", join)
				joins = append(joins, fmt.Sprintf("event_entry %s on event.id=%[1]s.event_id", joinAlias))
				clauses = append(clauses, fmt.Sprintf("%s.indexed=1 AND %[1]s.key=?", joinAlias))
				values = append(values, key)
				subclauses := []string{}
				for _, val := range vals {
					subclauses = append(subclauses, fmt.Sprintf("%s.value=?", joinAlias))
					values = append(values, val)
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

func (ei *EventIndex) RunMigration(from, to int) error {
	if from != 1 && to != 2 {
		return nil
	}

	log.Warnw("[migration v1=>v2] beginning transaction")

	tx, err := ei.db.Begin()
	if err != nil {
		return xerrors.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint

	if _, err := tx.Exec(`ALTER TABLE event_entry ADD COLUMN codec INTEGER`); err != nil {
		return xerrors.Errorf("failed to alter table event_entry to add codec column: %w", err)
	}

	log.Warnw("[migration v1=>v2] event_entry table schema updated")

	res, err := tx.Exec(`UPDATE event_entry SET codec = ? WHERE codec IS NULL`, 0x55)
	if err != nil {
		return xerrors.Errorf("failed to update table event_entry to set codec 0x55: %w", err)
	}
	n, _ := res.RowsAffected()
	log.Warnw("[migration v1=>v2] updated event entries with codec=0x55", "affected", n)

	var keyRewrites = [][]string{
		{"topic1", "t1"},
		{"topic2", "t2"},
		{"topic3", "t3"},
		{"topic4", "t4"},
		{"data", "d"},
	}

	for _, r := range keyRewrites {
		from, to := r[0], r[1]
		res, err := tx.Exec(`UPDATE event_entry SET key = ? WHERE key = ?`, to, from)
		if err != nil {
			return xerrors.Errorf("failed to update entries from key %s to key %s: %w", from, to, err)
		}
		n, _ := res.RowsAffected()
		log.Warnw("[migration v1=>v2] rewrote entry keys", "from", from, "to", to, "affected", n)
	}

	if _, err := tx.Exec(`DROP TABLE IF EXISTS event_entry_tmp`); err != nil {
		return xerrors.Errorf("failed to drop old event_entry_tmp temporary table: %w", err)
	}

	if _, err := tx.Exec(`CREATE TABLE event_entry_tmp (
		event_id INTEGER,
		indexed INTEGER NOT NULL,
		flags BLOB NOT NULL,
		key TEXT NOT NULL,
		codec INTEGER,
		value BLOB NOT NULL
	)`); err != nil {
		return xerrors.Errorf("failed to create temporary table: %w", err)
	}

	// Copy over all data. We will run the iterator from this table.
	if _, err := tx.Exec(`INSERT INTO event_entry_tmp SELECT * FROM event_entry`); err != nil {
		return xerrors.Errorf("failed to copy over data: %w", err)
	}

	// Add an index on value.
	if _, err := tx.Exec(`CREATE INDEX event_entry_value ON event_entry(value)`); err != nil {
		return xerrors.Errorf("failed to create index on event_entry: %w", err)
	}

	// Add an index on event_id.
	if _, err := tx.Exec(`CREATE INDEX event_entry_event_id ON event_entry(event_id)`); err != nil {
		return xerrors.Errorf("failed to create index on event_entry: %w", err)
	}

	// Match by value to be extra safe.
	update, err := tx.Prepare(`UPDATE event_entry SET value = ? WHERE event_id = ? and key = ?`)
	if err != nil {
		return xerrors.Errorf("failed to prepare update statement: %w", err)
	}

	log.Warnw("[migration v1=>v2] processing values")

	topics := map[string]struct{}{
		"t1": {},
		"t2": {},
		"t3": {},
		"t4": {},
	}

	// Pull the values out out of the original table, process them in Go land before updating, and the update the v2 table.
	rows, err := tx.Query(`SELECT event_id, key, value FROM event_entry_tmp`)
	if err != nil {
		return xerrors.Errorf("failed to select values: %w", err)
	}

	var i int
	for rows.Next() {
		var eventId int
		var key string
		var before []byte
		if err := rows.Scan(&eventId, &key, &before); err != nil {
			_ = rows.Close()
			return xerrors.Errorf("failed to scan from query: %w", err)
		}
		reader := bytes.NewReader(before)
		after, err := cbg.ReadByteArray(reader, 4<<20)
		if err != nil || reader.Len() > 0 {
			// Before Jan 23 we decoded events before putting them into the database. So
			// we're just going to assume that that's what's happening here, log an
			// error, and continue.
			after = before
		}
		if _, ok := topics[key]; ok && len(after) < 32 {
			// if this is a topic, leftpad to 32 bytes
			pvalue := make([]byte, 32)
			copy(pvalue[32-len(after):], after)
			after = pvalue
		}
		if _, err := update.Exec(after, eventId, key); err != nil {
			log.Warnw("failed to update entry", "value_before", before, "value_after", after)
		}

		if i++; i%500 == 0 {
			log.Warnw("[migration v1=>v2] processed values", "count", i)
		}
	}
	_ = rows.Close()

	log.Warnw("[migration v1=>v2] processed all values", "count", i)

	if _, err := tx.Exec(`DROP INDEX event_entry_value`); err != nil {
		log.Warnw("[migration v1=>v2] failed to drop index event_entry_value; ignoring")
	}

	if _, err := tx.Exec(`DROP INDEX event_entry_event_id`); err != nil {
		log.Warnw("[migration v1=>v2] failed to drop index event_entry_event_id; ignoring")
	}

	if _, err = tx.Exec(`DROP TABLE event_entry_tmp`); err != nil {
		return xerrors.Errorf("failed to drop original table: %w", err)
	}

	if _, err = tx.Exec(`INSERT INTO _meta (version) VALUES (2)`); err != nil {
		return xerrors.Errorf("failed to update schema version: %w", err)
	}

	log.Warnw("[migration v1=>v2] schema version updated to v2")

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("failed to commit transaction: %w", err)
	}

	log.Warnw("[migration v1=>v2] transaction committed; ALL DONE")

	return nil
}
