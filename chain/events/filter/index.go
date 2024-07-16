package filter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sqlite"
)

const DefaultDbFilename = "events.db"

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
}

var (
	log = logging.Logger("filter")
)

const (
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

func NewEventIndex(ctx context.Context, path string, chainStore *store.ChainStore) (*EventIndex, error) {
	db, _, err := sqlite.Open(path)
	if err != nil {
		return nil, xerrors.Errorf("failed to setup event index db: %w", err)
	}

	err = sqlite.InitDb(ctx, "event index", db, ddls, []sqlite.MigrationFunc{
		migrationVersion2(db, chainStore),
		migrationVersion3,
		migrationVersion4,
		migrationVersion5,
		migrationVersion6,
	})
	if err != nil {
		_ = db.Close()
		return nil, xerrors.Errorf("failed to setup event index db: %w", err)
	}

	eventIndex := EventIndex{db: db}

	if err = eventIndex.initStatements(); err != nil {
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

func (ei *EventIndex) IsHeightPast(ctx context.Context, height uint64) (bool, error) {
	maxHeight, err := ei.GetMaxHeightInIndex(ctx)
	if err != nil {
		return false, err
	}
	return height <= maxHeight, nil
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
