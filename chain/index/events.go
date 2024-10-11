package index

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/types"
)

type executedMessage struct {
	msg types.ChainMsg
	rct types.MessageReceipt
	// events extracted from receipt
	evs []types.Event
}

// events are indexed against their inclusion/message tipset when we get the corresponding execution tipset
func (si *SqliteIndexer) indexEvents(ctx context.Context, tx *sql.Tx, msgTs *types.TipSet, executionTs *types.TipSet) error {
	if si.idToRobustAddrFunc == nil {
		return xerrors.Errorf("indexer can not index events without an address resolver")
	}

	// check if we have an event indexed for any message in the `msgTs` tipset -> if so, there's nothig to do here
	// this makes event inserts idempotent
	msgTsKeyCidBytes, err := toTipsetKeyCidBytes(msgTs)
	if err != nil {
		return xerrors.Errorf("failed to get tipset key cid: %w", err)
	}

	// if we've already indexed events for this tipset, mark them as unreverted and return
	res, err := tx.Stmt(si.stmts.updateEventsToNonRevertedStmt).ExecContext(ctx, msgTsKeyCidBytes)
	if err != nil {
		return xerrors.Errorf("failed to unrevert events for tipset: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return xerrors.Errorf("failed to get rows affected by unreverting events for tipset: %w", err)
	}
	if rows > 0 {
		log.Infof("unreverted %d events for tipset: %s", rows, msgTs.Key())
		return nil
	}

	if !si.cs.IsStoringEvents() {
		return nil
	}

	ems, err := si.loadExecutedMessages(ctx, msgTs, executionTs)
	if err != nil {
		return xerrors.Errorf("failed to load executed messages: %w", err)
	}
	eventCount := 0
	addressLookups := make(map[abi.ActorID]address.Address)

	for _, em := range ems {
		msgCidBytes := em.msg.Cid().Bytes()

		// read message id for this message cid and tipset key cid
		var messageID int64
		if err := tx.Stmt(si.stmts.getMsgIdForMsgCidAndTipsetStmt).QueryRowContext(ctx, msgTsKeyCidBytes, msgCidBytes).Scan(&messageID); err != nil {
			return xerrors.Errorf("failed to get message id for message cid and tipset key cid: %w", err)
		}
		if messageID == 0 {
			return xerrors.Errorf("message id not found for message cid %s and tipset key cid %s", em.msg.Cid(), msgTs.Key())
		}

		// Insert events for this message
		for _, event := range em.evs {
			addr, found := addressLookups[event.Emitter]
			if !found {
				var ok bool
				addr, ok = si.idToRobustAddrFunc(ctx, event.Emitter, executionTs)
				if !ok {
					// not an address we will be able to match against
					continue
				}
				addressLookups[event.Emitter] = addr
			}

			robustAddrbytes := addr.Bytes()
			if addr.Protocol() != address.Delegated {
				robustAddrbytes = nil
			}

			// Insert event into events table
			eventResult, err := tx.Stmt(si.stmts.insertEventStmt).ExecContext(ctx, messageID, eventCount, uint64(event.Emitter), robustAddrbytes, 0)
			if err != nil {
				return xerrors.Errorf("failed to insert event: %w", err)
			}

			// Get the event_id of the inserted event
			eventID, err := eventResult.LastInsertId()
			if err != nil {
				return xerrors.Errorf("failed to get last insert id for event: %w", err)
			}

			// Insert event entries
			for _, entry := range event.Entries {
				_, err := tx.Stmt(si.stmts.insertEventEntryStmt).ExecContext(ctx,
					eventID,
					isIndexedValue(entry.Flags),
					[]byte{entry.Flags},
					entry.Key,
					entry.Codec,
					entry.Value,
				)
				if err != nil {
					return xerrors.Errorf("failed to insert event entry: %w", err)
				}
			}
			eventCount++
		}
	}

	return nil
}

func (si *SqliteIndexer) loadExecutedMessages(ctx context.Context, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
	msgs, err := si.cs.MessagesForTipset(ctx, msgTs)
	if err != nil {
		return nil, xerrors.Errorf("failed to get messages for tipset: %w", err)
	}

	st := si.cs.ActorStore(ctx)

	receiptsArr, err := blockadt.AsArray(st, rctTs.Blocks()[0].ParentMessageReceipts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load message receipts array: %w", err)
	}

	if uint64(len(msgs)) != receiptsArr.Length() {
		return nil, xerrors.Errorf("mismatching message and receipt counts (%d msgs, %d rcts)", len(msgs), receiptsArr.Length())
	}

	ems := make([]executedMessage, len(msgs))

	for i := 0; i < len(msgs); i++ {
		ems[i].msg = msgs[i]

		var rct types.MessageReceipt
		found, err := receiptsArr.Get(uint64(i), &rct)
		if err != nil {
			return nil, xerrors.Errorf("failed to load receipt %d: %w", i, err)
		}
		if !found {
			return nil, xerrors.Errorf("receipt %d not found", i)
		}
		ems[i].rct = rct

		// no events in the receipt
		if rct.EventsRoot == nil {
			continue
		}

		eventsArr, err := amt4.LoadAMT(ctx, st, *rct.EventsRoot, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
		if err != nil {
			if si.recomputeTipSetStateFunc == nil {
				return nil, xerrors.Errorf("failed to load events amt for message %s: %w", ems[i].msg.Cid(), err)
			}
			log.Warnf("failed to load events amt for message %s: %s; recomputing tipset state to regenerate events", ems[i].msg.Cid(), err)

			if err := si.recomputeTipSetStateFunc(ctx, msgTs); err != nil {
				return nil, xerrors.Errorf("failed to recompute missing events; failed to recompute tipset state: %w", err)
			}

			eventsArr, err = amt4.LoadAMT(ctx, st, *rct.EventsRoot, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
			if err != nil {
				return nil, xerrors.Errorf("failed to load events amt for message %s: %w", ems[i].msg.Cid(), err)
			}

			log.Infof("successfully recomputed tipset state and loaded events amt for message %s", ems[i].msg.Cid())
		}

		ems[i].evs = make([]types.Event, eventsArr.Len())
		var evt types.Event
		err = eventsArr.ForEach(ctx, func(u uint64, deferred *cbg.Deferred) error {
			if u > math.MaxInt {
				return xerrors.Errorf("too many events")
			}
			if err := evt.UnmarshalCBOR(bytes.NewReader(deferred.Raw)); err != nil {
				return err
			}

			cpy := evt
			ems[i].evs[int(u)] = cpy
			return nil
		})

		if err != nil {
			return nil, xerrors.Errorf("failed to iterate over events for message %d: %w", i, err)
		}

	}

	return ems, nil
}

// checkTipsetIndexedStatus verifies if a specific tipset is indexed based on the EventFilter.
// It returns nil if the tipset is indexed, ErrNotFound if it's not indexed or not specified,
func (si *SqliteIndexer) checkTipsetIndexedStatus(ctx context.Context, f *EventFilter) error {
	var tipsetKeyCid []byte
	var err error

	// Determine the tipset to check based on the filter
	switch {
	case f.TipsetCid != cid.Undef:
		tipsetKeyCid = f.TipsetCid.Bytes()
	case f.MinHeight >= 0 && f.MinHeight == f.MaxHeight:
		tipsetKeyCid, err = si.getTipsetKeyCidByHeight(ctx, f.MinHeight)
		if err != nil {
			if err == ErrNotFound {
				// this means that this is a null round and there exist no events for this epoch
				return nil
			}

			return xerrors.Errorf("failed to get tipset key cid by height: %w", err)
		}
	default:
		// Filter doesn't specify a specific tipset
		return nil
	}

	// If we couldn't determine a specific tipset, return ErrNotFound
	if tipsetKeyCid == nil {
		return ErrNotFound
	}

	// Check if the determined tipset is indexed
	exists, err := si.isTipsetIndexed(ctx, tipsetKeyCid)
	if err != nil {
		return xerrors.Errorf("failed to check if tipset is indexed: %w", err)
	}

	if exists {
		return nil // Tipset is indexed
	}

	return ErrNotFound // Tipset is not indexed
}

// getTipsetKeyCidByHeight retrieves the tipset key CID for a given height.
func (si *SqliteIndexer) getTipsetKeyCidByHeight(ctx context.Context, height abi.ChainEpoch) ([]byte, error) {
	ts, err := si.cs.GetTipsetByHeight(ctx, height, nil, false)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset by height: %w", err)
	}

	if ts.Height() != height {
		// this means that this is a null round
		return nil, ErrNotFound
	}

	return toTipsetKeyCidBytes(ts)
}

// GetEventsForFilter returns matching events for the given filter
// Returns nil, nil if the filter has no matching events
// Returns nil, ErrNotFound if the filter has no matching events and the tipset is not indexed
// Returns nil, err for all other errors
func (si *SqliteIndexer) GetEventsForFilter(ctx context.Context, f *EventFilter, excludeReverted bool) ([]*CollectedEvent, error) {
	getEventsFnc := func(stmt *sql.Stmt, values []any) ([]*CollectedEvent, error) {
		q, err := stmt.QueryContext(ctx, values...)
		if err != nil {
			return nil, xerrors.Errorf("failed to query events: %w", err)
		}
		defer func() { _ = q.Close() }()

		var ces []*CollectedEvent
		var currentID int64 = -1
		var ce *CollectedEvent

		for q.Next() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			var row struct {
				id           int64
				height       uint64
				tipsetKeyCid []byte
				emitterID    uint64
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
				&row.tipsetKeyCid,
				&row.emitterID,
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
				return nil, xerrors.Errorf("read prefill row: %w", err)
			}

			if row.id != currentID {
				if ce != nil {
					ces = append(ces, ce)
					ce = nil
					// Unfortunately we can't easily incorporate the max results limit into the query due to the
					// unpredictable number of rows caused by joins
					// Break here to stop collecting rows
					if f.MaxResults > 0 && len(ces) >= f.MaxResults {
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

				if row.emitterAddr == nil {
					ce.EmitterAddr, err = address.NewIDAddress(row.emitterID)
					if err != nil {
						return nil, xerrors.Errorf("failed to parse emitter id: %w", err)
					}
				} else {
					ce.EmitterAddr, err = address.NewFromBytes(row.emitterAddr)
					if err != nil {
						return nil, xerrors.Errorf("parse emitter addr: %w", err)
					}
				}

				tsKeyCid, err := cid.Cast(row.tipsetKeyCid)
				if err != nil {
					return nil, xerrors.Errorf("parse tipsetkey cid: %w", err)
				}

				ts, err := si.cs.GetTipSetByCid(ctx, tsKeyCid)
				if err != nil {
					return nil, xerrors.Errorf("get tipset by cid: %w", err)
				}

				ce.TipSetKey = ts.Key()

				ce.MsgCid, err = cid.Cast(row.messageCid)
				if err != nil {
					return nil, xerrors.Errorf("parse message cid: %w", err)
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
			return nil, nil
		}

		// collected event list is in inverted order since we selected only the most recent events
		// sort it into height order
		sort.Slice(ces, func(i, j int) bool { return ces[i].Height < ces[j].Height })

		return ces, nil
	}

	values, query, err := makePrefillFilterQuery(f, excludeReverted)
	if err != nil {
		return nil, xerrors.Errorf("failed to make prefill filter query: %w", err)
	}

	stmt, err := si.db.Prepare(query)
	if err != nil {
		return nil, xerrors.Errorf("prepare prefill query: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	ces, err := getEventsFnc(stmt, values)
	if err != nil {
		return nil, xerrors.Errorf("failed to get events: %w", err)
	}
	if len(ces) == 0 {
		// there's no matching events for the filter, wait till index has caught up to the head and then retry
		if err := si.waitTillHeadIndexed(ctx); err != nil {
			return nil, xerrors.Errorf("failed to wait for head to be indexed: %w", err)
		}
		ces, err = getEventsFnc(stmt, values)
		if err != nil {
			return nil, xerrors.Errorf("failed to get events: %w", err)
		}

		if len(ces) == 0 {
			return nil, si.checkTipsetIndexedStatus(ctx, f)
		}
	}

	return ces, nil
}

func makePrefillFilterQuery(f *EventFilter, excludeReverted bool) ([]any, string, error) {
	clauses := []string{}
	values := []any{}
	joins := []string{}

	if f.TipsetCid != cid.Undef {
		clauses = append(clauses, "tm.tipset_key_cid=?")
		values = append(values, f.TipsetCid.Bytes())
	} else {
		if f.MinHeight >= 0 && f.MinHeight == f.MaxHeight {
			clauses = append(clauses, "tm.height=?")
			values = append(values, f.MinHeight)
		} else {
			if f.MaxHeight >= 0 && f.MinHeight >= 0 {
				clauses = append(clauses, "tm.height BETWEEN ? AND ?")
				values = append(values, f.MinHeight, f.MaxHeight)
			} else if f.MinHeight >= 0 {
				clauses = append(clauses, "tm.height >= ?")
				values = append(values, f.MinHeight)
			} else if f.MaxHeight >= 0 {
				clauses = append(clauses, "tm.height <= ?")
				values = append(values, f.MaxHeight)
			}
		}
	}

	if excludeReverted {
		clauses = append(clauses, "e.reverted=?")
		values = append(values, false)
	}

	if len(f.Addresses) > 0 {
		idAddresses := make([]uint64, 0)
		delegatedAddresses := make([][]byte, 0)

		for _, addr := range f.Addresses {
			switch addr.Protocol() {
			case address.ID:
				id, err := address.IDFromAddress(addr)
				if err != nil {
					return nil, "", xerrors.Errorf("failed to get ID from address: %w", err)
				}
				idAddresses = append(idAddresses, id)
			case address.Delegated:
				delegatedAddresses = append(delegatedAddresses, addr.Bytes())
			default:
				return nil, "", xerrors.Errorf("can only query events by ID or Delegated addresses; but request has address: %s", addr)
			}
		}

		if len(idAddresses) > 0 {
			placeholders := strings.Repeat("?,", len(idAddresses)-1) + "?"
			clauses = append(clauses, "e.emitter_id IN ("+placeholders+")")
			for _, id := range idAddresses {
				values = append(values, id)
			}
		}

		if len(delegatedAddresses) > 0 {
			placeholders := strings.Repeat("?,", len(delegatedAddresses)-1) + "?"
			clauses = append(clauses, "e.emitter_addr IN ("+placeholders+")")
			for _, addr := range delegatedAddresses {
				values = append(values, addr)
			}
		}
	}

	if len(f.KeysWithCodec) > 0 {
		join := 0
		for key, vals := range f.KeysWithCodec {
			if len(vals) > 0 {
				join++
				joinAlias := fmt.Sprintf("ee%d", join)
				joins = append(joins, fmt.Sprintf("event_entry %s ON e.event_id=%[1]s.event_id", joinAlias))
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
			e.event_id,
			tm.height,
			tm.tipset_key_cid,
			e.emitter_id,
			e.emitter_addr,
			e.event_index,
			tm.message_cid,
			tm.message_index,
			e.reverted,
			ee.flags,
			ee.key,
			ee.codec,
			ee.value
		FROM event e
		JOIN tipset_message tm ON e.message_id = tm.message_id
		JOIN event_entry ee ON e.event_id = ee.event_id`

	if len(joins) > 0 {
		s = s + ", " + strings.Join(joins, ", ")
	}

	if len(clauses) > 0 {
		s = s + " WHERE " + strings.Join(clauses, " AND ")
	}

	// retain insertion order of event_entry rows
	s += " ORDER BY tm.height DESC, ee._rowid_ ASC"
	return values, s, nil
}
