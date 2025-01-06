package index

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/types"
)

const maxLookBackForWait = 120 // one hour of tipsets

var (
	ErrMaxResultsReached = xerrors.New("filter matches too many events, try a more restricted filter")
)

type ErrRangeInFuture struct {
	HighestEpoch int
}

func (e *ErrRangeInFuture) Error() string {
	return fmt.Sprintf("range end is in the future, highest epoch: %d", e.HighestEpoch)
}

func (e *ErrRangeInFuture) Is(target error) bool {
	_, ok := target.(*ErrRangeInFuture)
	return ok
}

type executedMessage struct {
	msg types.ChainMsg
	rct types.MessageReceipt
	// events extracted from receipt
	evs []types.Event
}

// events are indexed against their inclusion/message tipset when we get the corresponding execution tipset
func (si *SqliteIndexer) indexEvents(ctx context.Context, tx *sql.Tx, msgTs *types.TipSet, executionTs *types.TipSet) error {
	if si.actorToDelegatedAddresFunc == nil {
		return xerrors.Errorf("indexer can not index events without an address resolver")
	}
	if si.executedMessagesLoaderFunc == nil {
		return xerrors.Errorf("indexer can not index events without an event loader")
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
		log.Debugf("unreverted %d events for tipset: %s", rows, msgTs.Key())
		return nil
	}

	if !si.cs.IsStoringEvents() {
		return nil
	}

	ems, err := si.executedMessagesLoaderFunc(ctx, si.cs, msgTs, executionTs)
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
				addr, ok = si.actorToDelegatedAddresFunc(ctx, event.Emitter, executionTs)
				if !ok {
					// not an address we will be able to match against
					continue
				}
				addressLookups[event.Emitter] = addr
			}

			var robustAddrbytes []byte
			if addr.Protocol() == address.Delegated {
				robustAddrbytes = addr.Bytes()
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
					isIndexedFlag(entry.Flags),
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

func loadExecutedMessages(ctx context.Context, cs ChainStore, recomputeTipSetStateFunc RecomputeTipSetStateFunc, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
	msgs, err := cs.MessagesForTipset(ctx, msgTs)
	if err != nil {
		return nil, xerrors.Errorf("failed to get messages for tipset: %w", err)
	}

	st := cs.ActorStore(ctx)

	var recomputed bool
	recompute := func() error {
		tskCid, err2 := rctTs.Key().Cid()
		if err2 != nil {
			return xerrors.Errorf("failed to compute tipset key cid: %w", err2)
		}

		log.Warnf("failed to load receipts for tipset %s (height %d): %s; recomputing tipset state", tskCid.String(), rctTs.Height(), err.Error())
		if err := recomputeTipSetStateFunc(ctx, msgTs); err != nil {
			return xerrors.Errorf("failed to recompute tipset state: %w", err)
		}
		log.Warnf("successfully recomputed tipset state and loaded events for %s (height %d)", tskCid.String(), rctTs.Height())
		return nil
	}

	receiptsArr, err := blockadt.AsArray(st, rctTs.Blocks()[0].ParentMessageReceipts)
	if err != nil {
		if !ipld.IsNotFound(err) || recomputeTipSetStateFunc == nil {
			return nil, xerrors.Errorf("failed to load message receipts: %w", err)
		}

		if err := recompute(); err != nil {
			return nil, err
		}
		recomputed = true
		receiptsArr, err = blockadt.AsArray(st, rctTs.Blocks()[0].ParentMessageReceipts)
		if err != nil {
			return nil, xerrors.Errorf("failed to load receipts after tipset state recompute: %w", err)
		}
	}

	if uint64(len(msgs)) != receiptsArr.Length() {
		return nil, xerrors.Errorf("mismatching message and receipt counts (%d msgs, %d rcts)", len(msgs), receiptsArr.Length())
	}

	ems := make([]executedMessage, len(msgs))

	for i := 0; i < len(msgs); i++ {
		ems[i].msg = msgs[i]

		var rct types.MessageReceipt
		if found, err := receiptsArr.Get(uint64(i), &rct); err != nil {
			return nil, xerrors.Errorf("failed to load receipt %d: %w", i, err)
		} else if !found {
			return nil, xerrors.Errorf("receipt %d not found", i)
		}
		ems[i].rct = rct

		// no events in the receipt
		if rct.EventsRoot == nil {
			continue
		}

		eventsArr, err := amt4.LoadAMT(ctx, st, *rct.EventsRoot, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
		if err != nil {
			if !ipld.IsNotFound(err) || recomputeTipSetStateFunc == nil || recomputed {
				return nil, xerrors.Errorf("failed to load events root for message %s: err: %w", ems[i].msg.Cid(), err)
			}
			// we may have the receipts but not the events, IsStoringEvents may have been false
			if err := recompute(); err != nil {
				return nil, err
			}
			eventsArr, err = amt4.LoadAMT(ctx, st, *rct.EventsRoot, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
			if err != nil {
				return nil, xerrors.Errorf("failed to load events amt for re-executed tipset for message %s: %w", ems[i].msg.Cid(), err)
			}
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

// checkFilterTipsetsIndexed verifies if a tipset, or a range of tipsets, specified by a given
// filter is indexed. It checks for the existence of non-null rounds at the range boundaries.
func (si *SqliteIndexer) checkFilterTipsetsIndexed(ctx context.Context, f *EventFilter) error {
	// Three cases to consider:
	// 1. Specific tipset is provided
	// 2. Single tipset is specified by the height range (min=max)
	// 3. Range of tipsets is specified by the height range (min!=max)
	// We'll handle the first two cases here and the third case in checkRangeIndexedStatus

	var tipsetKeyCid []byte
	var err error

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
		return si.checkRangeIndexedStatus(ctx, f.MinHeight, f.MaxHeight)
	}

	// If we couldn't determine a specific tipset, return ErrNotFound
	if tipsetKeyCid == nil {
		return ErrNotFound
	}

	// Check if the determined tipset is indexed
	if exists, err := si.isTipsetIndexed(ctx, tipsetKeyCid); err != nil {
		return xerrors.Errorf("failed to check if tipset is indexed: %w", err)
	} else if exists {
		return nil // Tipset is indexed
	}

	return ErrNotFound // Tipset is not indexed
}

// checkRangeIndexedStatus verifies if a range of tipsets specified by the given height range is
// indexed. It checks for the existence of non-null rounds at the range boundaries.
func (si *SqliteIndexer) checkRangeIndexedStatus(ctx context.Context, minHeight abi.ChainEpoch, maxHeight abi.ChainEpoch) error {
	head := si.cs.GetHeaviestTipSet()
	if minHeight > head.Height() || maxHeight > head.Height() {
		return &ErrRangeInFuture{HighestEpoch: int(head.Height())}
	}

	// Find the first non-null round in the range
	startCid, startHeight, err := si.findFirstNonNullRound(ctx, minHeight, maxHeight)
	if err != nil {
		return xerrors.Errorf("failed to find first non-null round: %w", err)
	}
	// If all rounds are null, consider the range valid
	if startCid == nil {
		return nil
	}

	// Find the last non-null round in the range
	endCid, endHeight, err := si.findLastNonNullRound(ctx, maxHeight, minHeight)
	if err != nil {
		return xerrors.Errorf("failed to find last non-null round: %w", err)
	}
	// We should have already rulled out all rounds being null in the startCid check
	if endCid == nil {
		return xerrors.Errorf("unexpected error finding last non-null round: all rounds are null but start round is not (%d to %d)", minHeight, maxHeight)
	}

	// Check indexing status for start and end tipsets
	if err := si.checkTipsetIndexedStatus(ctx, startCid, startHeight); err != nil {
		return err
	}
	if err := si.checkTipsetIndexedStatus(ctx, endCid, endHeight); err != nil {
		return err
	}
	// Assume (not necessarily correctly, but likely) that all tipsets within the range are indexed

	return nil
}

func (si *SqliteIndexer) checkTipsetIndexedStatus(ctx context.Context, tipsetKeyCid []byte, height abi.ChainEpoch) error {
	exists, err := si.isTipsetIndexed(ctx, tipsetKeyCid)
	if err != nil {
		return xerrors.Errorf("failed to check if tipset at epoch %d is indexed: %w", height, err)
	} else if exists {
		return nil // has been indexed
	}
	return ErrNotFound
}

// findFirstNonNullRound finds the first non-null round starting from minHeight up to maxHeight.
// It updates the minHeight to the found height and returns the tipset key CID.
func (si *SqliteIndexer) findFirstNonNullRound(ctx context.Context, minHeight abi.ChainEpoch, maxHeight abi.ChainEpoch) ([]byte, abi.ChainEpoch, error) {
	for height := minHeight; height <= maxHeight; height++ {
		cid, err := si.getTipsetKeyCidByHeight(ctx, height)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return nil, 0, xerrors.Errorf("failed to get tipset key cid for height %d: %w", height, err)
			}
			// else null round, keep searching
			continue
		}
		return cid, height, nil
	}
	// All rounds are null
	return nil, 0, nil
}

// findLastNonNullRound finds the last non-null round starting from maxHeight down to minHeight
func (si *SqliteIndexer) findLastNonNullRound(ctx context.Context, maxHeight abi.ChainEpoch, minHeight abi.ChainEpoch) ([]byte, abi.ChainEpoch, error) {
	for height := maxHeight; height >= minHeight; height-- {
		cid, err := si.getTipsetKeyCidByHeight(ctx, height)
		if err == nil {
			return cid, height, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, 0, xerrors.Errorf("failed to get tipset key cid for height %d: %w", height, err)
		}
	}
	// All rounds are null
	return nil, 0, nil
}

// getTipsetKeyCidByHeight retrieves the tipset key CID for a given height from the ChainStore
func (si *SqliteIndexer) getTipsetKeyCidByHeight(ctx context.Context, height abi.ChainEpoch) ([]byte, error) {
	ts, err := si.cs.GetTipsetByHeight(ctx, height, nil, false)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset by height: %w", err)
	}
	if ts == nil {
		return nil, xerrors.Errorf("tipset is nil for height: %d", height)
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
func (si *SqliteIndexer) GetEventsForFilter(ctx context.Context, f *EventFilter) ([]*CollectedEvent, error) {
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

			// The query will return all entries for all matching events, so we need to keep track
			// of which event we are dealing with and create a new one each time we see a new id
			if row.id != currentID {
				// Unfortunately we can't easily incorporate the max results limit into the query due to the
				// unpredictable number of rows caused by joins
				// Error here to inform the caller that we've hit the max results limit
				if f.MaxResults > 0 && len(ces) >= f.MaxResults {
					return nil, ErrMaxResultsReached
				}

				currentID = row.id
				ce = &CollectedEvent{
					EventIdx: row.eventIndex,
					Reverted: row.reverted,
					Height:   abi.ChainEpoch(row.height),
					MsgIdx:   row.messageIndex,
				}
				ces = append(ces, ce)

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
				if ts == nil {
					return nil, xerrors.Errorf("failed to get tipset from cid: tipset is nil for cid: %s", tsKeyCid)
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

		if len(ces) == 0 {
			return nil, nil
		}

		return ces, nil
	}

	values, query, err := makePrefillFilterQuery(f)
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
		height := f.MaxHeight
		if f.TipsetCid != cid.Undef {
			ts, err := si.cs.GetTipSetByCid(ctx, f.TipsetCid)
			if err != nil {
				return nil, xerrors.Errorf("failed to get tipset by cid: %w", err)
			}
			if ts == nil {
				return nil, xerrors.Errorf("failed to get tipset from cid: tipset is nil for cid: %s", f.TipsetCid)
			}
			height = ts.Height()
		}
		if height > 0 {
			head := si.cs.GetHeaviestTipSet()
			if head == nil {
				return nil, xerrors.New("failed to get head: head is nil")
			}
			headHeight := head.Height()
			maxLookBackHeight := headHeight - maxLookBackForWait

			// if the height is old enough, we'll assume the index is caught up to it and not bother
			// waiting for it to be indexed
			if height <= maxLookBackHeight {
				return nil, si.checkFilterTipsetsIndexed(ctx, f)
			}
		}

		// there's no matching events for the filter, wait till index has caught up to the head and then retry
		if err := si.waitTillHeadIndexed(ctx); err != nil {
			return nil, xerrors.Errorf("failed to wait for head to be indexed: %w", err)
		}
		ces, err = getEventsFnc(stmt, values)
		if err != nil {
			return nil, xerrors.Errorf("failed to get events: %w", err)
		}

		if len(ces) == 0 {
			return nil, si.checkFilterTipsetsIndexed(ctx, f)
		}
	}

	return ces, nil
}

func makePrefillFilterQuery(f *EventFilter) ([]any, string, error) {
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
			} else {
				return nil, "", xerrors.Errorf("filter must specify either a tipset or a height range")
			}
		}
		// unless asking for a specific tipset, we never want to see reverted historical events
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
				joins = append(joins, fmt.Sprintf("event_entry %s ON e.id=%[1]s.event_id", joinAlias))
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
	} else if f.Codec != 0 { // if no keys are specified, we can use the codec filter
		clauses = append(clauses, "ee.codec=?")
		values = append(values, f.Codec)
	}

	s := `SELECT
			e.id,
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
		JOIN tipset_message tm ON e.message_id = tm.id
		JOIN event_entry ee ON e.id = ee.event_id`

	if len(joins) > 0 {
		s = s + ", " + strings.Join(joins, ", ")
	}

	if len(clauses) > 0 {
		s = s + " WHERE " + strings.Join(clauses, " AND ")
	}

	// retain insertion order of event_entry rows
	s += " ORDER BY tm.height ASC, tm.message_index ASC, e.event_index ASC, ee._rowid_ ASC"
	return values, s, nil
}
