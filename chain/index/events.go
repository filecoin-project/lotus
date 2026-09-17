package index

import (
	"bytes"
	"context"
	"database/sql"
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
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

const maxLookBackForWait = 120 // one hour of tipsets

// Avoid an unbounded eager allocation when a trusted caller configures a very
// large event range. The default public range (2,880 epochs) still fits in the
// hint; larger ranges grow the map normally as the canonical chain is walked.
const maxEventRangePreallocation = 8192

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
	blockBloom := ethtypes.NewEmptyEthBloom()

	if rows > 0 {
		log.Debugf("unreverted %d events for tipset: %s", rows, msgTs.Key())
		hasBloom, err := si.hasTipsetBloom(ctx, tx, msgTsKeyCidBytes)
		if err != nil {
			return xerrors.Errorf("failed to check tipset bloom: %w", err)
		}
		if hasBloom {
			return nil
		}
		if err := si.buildTipsetBloomFromIndex(ctx, tx, msgTsKeyCidBytes, blockBloom); err != nil {
			return xerrors.Errorf("failed to build tipset bloom from index: %w", err)
		}
		if err := si.upsertTipsetBloom(ctx, tx, msgTsKeyCidBytes, msgTs.Height(), blockBloom); err != nil {
			return xerrors.Errorf("failed to store tipset bloom: %w", err)
		}
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
	messageIDs := make(map[string]int64)

	if err := si.forEachExecutedEvent(ctx, ems, executionTs, func(em executedMessage, event types.Event, addr address.Address) error {
		msgCid := em.msg.Cid()
		messageID, found := messageIDs[msgCid.KeyString()]
		if !found {
			msgCidBytes := msgCid.Bytes()
			if err := tx.Stmt(si.stmts.getMsgIdForMsgCidAndTipsetStmt).QueryRowContext(ctx, msgTsKeyCidBytes, msgCidBytes).Scan(&messageID); err != nil {
				return xerrors.Errorf("failed to get message id for message cid and tipset key cid: %w", err)
			}
			if messageID == 0 {
				return xerrors.Errorf("message id not found for message cid %s and tipset key cid %s", msgCid, msgTs.Key())
			}
			messageIDs[msgCid.KeyString()] = messageID
		}

		addEventToBloom(blockBloom, addr, event.Entries)

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
		return nil
	}); err != nil {
		return err
	}

	if err := si.upsertTipsetBloom(ctx, tx, msgTsKeyCidBytes, msgTs.Height(), blockBloom); err != nil {
		return xerrors.Errorf("failed to store tipset bloom: %w", err)
	}

	return nil
}

func (si *SqliteIndexer) forEachExecutedEvent(ctx context.Context, ems []executedMessage, executionTs *types.TipSet, cb func(executedMessage, types.Event, address.Address) error) error {
	addressLookups := make(map[abi.ActorID]address.Address)
	for _, em := range ems {
		for _, event := range em.evs {
			addr, found := addressLookups[event.Emitter]
			if !found {
				var ok bool
				addr, ok = si.actorToDelegatedAddresFunc(ctx, event.Emitter, executionTs)
				if !ok {
					continue
				}
				addressLookups[event.Emitter] = addr
			}
			if err := cb(em, event, addr); err != nil {
				return err
			}
		}
	}
	return nil
}

func (si *SqliteIndexer) buildTipsetBloomFromIndex(ctx context.Context, tx *sql.Tx, tipsetKeyCid []byte, blockBloom ethtypes.EthBytes) error {
	rows, err := tx.Stmt(si.stmts.getTipsetEventEntriesStmt).QueryContext(ctx, tipsetKeyCid)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var currentID int64
	var emitterAddr address.Address
	var entries []types.EventEntry
	flush := func() error {
		if currentID == 0 {
			return nil
		}
		addEventToBloom(blockBloom, emitterAddr, entries)
		return nil
	}

	for rows.Next() {
		var (
			eventID          int64
			emitterID        uint64
			emitterAddrBytes []byte
			flags            []byte
			key              string
			codec            uint64
			value            []byte
		)
		if err := rows.Scan(&eventID, &emitterID, &emitterAddrBytes, &flags, &key, &codec, &value); err != nil {
			return xerrors.Errorf("read indexed event row: %w", err)
		}
		if len(flags) == 0 {
			return xerrors.Errorf("event entry for event %d has no flags", eventID)
		}
		if eventID != currentID {
			if err := flush(); err != nil {
				return err
			}
			currentID = eventID
			entries = entries[:0]
			if emitterAddrBytes == nil {
				emitterAddr, err = address.NewIDAddress(emitterID)
				if err != nil {
					return xerrors.Errorf("failed to parse emitter id: %w", err)
				}
			} else {
				emitterAddr, err = address.NewFromBytes(emitterAddrBytes)
				if err != nil {
					return xerrors.Errorf("parse emitter addr: %w", err)
				}
			}
		}
		entries = append(entries, types.EventEntry{
			Flags: flags[0],
			Key:   key,
			Codec: codec,
			Value: value,
		})
	}
	if err := rows.Err(); err != nil {
		return xerrors.Errorf("read indexed event rows: %w", err)
	}
	return flush()
}

func addEventToBloom(blockBloom ethtypes.EthBytes, emitterAddr address.Address, entries []types.EventEntry) {
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(emitterAddr)
	if err != nil {
		log.Warnw("failed to convert event emitter address to Ethereum address for bloom", "address", emitterAddr, "error", err)
		return
	}

	_, topics, ok := ethtypes.EthLogFromEvent(entries)
	if !ok {
		return
	}

	ethtypes.EthBloomSet(blockBloom, ethAddr[:])
	for _, topic := range topics {
		ethtypes.EthBloomSet(blockBloom, topic[:])
	}
}

func (si *SqliteIndexer) upsertTipsetBloom(ctx context.Context, tx *sql.Tx, tipsetKeyCid []byte, height abi.ChainEpoch, bloom ethtypes.EthBytes) error {
	_, err := tx.Stmt(si.stmts.insertTipsetBloomStmt).ExecContext(ctx, tipsetKeyCid, height, []byte(bloom))
	return err
}

func (si *SqliteIndexer) hasTipsetBloom(ctx context.Context, tx *sql.Tx, tipsetKeyCid []byte) (bool, error) {
	var hasBloom bool
	err := tx.Stmt(si.stmts.hasTipsetBloomStmt).QueryRowContext(ctx, tipsetKeyCid).Scan(&hasBloom)
	return hasBloom, err
}

func loadExecutedMessages(ctx context.Context, cs ChainStore, recomputeTipSetStateFunc RecomputeTipSetStateFunc, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
	msgs, err := cs.MessagesForTipset(ctx, msgTs)
	if err != nil {
		return nil, xerrors.Errorf("failed to get messages for tipset: %w", err)
	}

	st := cs.ActorStore(ctx)

	var recomputed bool
	recompute := func(loadErr error) error {
		tskCid, err2 := rctTs.Key().Cid()
		if err2 != nil {
			return xerrors.Errorf("failed to compute tipset key cid: %w", err2)
		}

		log.Warnf("failed to load receipts for tipset %s (height %d): %s; recomputing tipset state", tskCid.String(), rctTs.Height(), loadErr.Error())
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

		if err := recompute(err); err != nil {
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
			if err := recompute(err); err != nil {
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

type eventRangeCoverage struct {
	minHeight abi.ChainEpoch
	maxHeight abi.ChainEpoch
	tipsets   map[cid.Cid]struct{}
}

func (si *SqliteIndexer) getEventRangeCoverage(ctx context.Context, minHeight, maxHeight abi.ChainEpoch) (*eventRangeCoverage, error) {
	head := si.cs.GetHeaviestTipSet()
	if head == nil {
		return nil, xerrors.New("failed to get head: head is nil")
	}

	if minHeight < 0 && maxHeight < 0 {
		return nil, xerrors.New("filter must specify a minimum or maximum height")
	}
	if minHeight < 0 {
		minHeight = 0
	}
	if maxHeight < 0 {
		// Events from the heaviest tipset are not available until its child is
		// applied, so an open range is pinned to the captured head's parent.
		maxHeight = head.Height() - 1
	}
	if minHeight > head.Height() || maxHeight > head.Height() {
		return nil, &ErrRangeInFuture{HighestEpoch: int(head.Height())}
	}

	tipsetCapacity := 0
	if maxHeight >= minHeight && maxHeight >= 0 {
		span := maxHeight - minHeight
		if span < maxEventRangePreallocation {
			tipsetCapacity = int(span) + 1
		} else {
			tipsetCapacity = maxEventRangePreallocation
		}
	}
	coverage := &eventRangeCoverage{
		minHeight: minHeight,
		maxHeight: maxHeight,
		tipsets:   make(map[cid.Cid]struct{}, tipsetCapacity),
	}
	if maxHeight < minHeight || maxHeight < 0 {
		return coverage, nil
	}

	ts, err := si.cs.GetTipsetByHeight(ctx, maxHeight, head, true)
	if err != nil {
		return nil, xerrors.Errorf("failed to get canonical tipset at or before height %d: %w", maxHeight, err)
	}
	if ts == nil {
		return nil, xerrors.Errorf("failed to get canonical tipset at or before height %d: tipset is nil", maxHeight)
	}
	tsKey := ts.Key()

	for ts.Height() >= minHeight {
		if ts.Height() > maxHeight {
			return nil, xerrors.Errorf("canonical tipset height %d is above requested maximum %d", ts.Height(), maxHeight)
		}

		tsKeyCid, err := tsKey.Cid()
		if err != nil {
			return nil, xerrors.Errorf("failed to get tipset key cid at height %d: %w", ts.Height(), err)
		}
		coverage.tipsets[tsKeyCid] = struct{}{}

		if ts.Height() == 0 || ts.Height() == minHeight {
			break
		}
		parentKey := ts.Parents()
		parent, err := si.cs.GetTipSetFromKey(ctx, parentKey)
		if err != nil {
			return nil, xerrors.Errorf("failed to get parent of tipset at height %d: %w", ts.Height(), err)
		}
		if parent == nil {
			return nil, xerrors.Errorf("failed to get parent of tipset at height %d: tipset is nil", ts.Height())
		}
		if parent.Height() >= ts.Height() {
			return nil, xerrors.Errorf("invalid chain order: parent height %d is not below child height %d", parent.Height(), ts.Height())
		}
		ts = parent
		tsKey = parentKey
	}

	return coverage, nil
}

func (si *SqliteIndexer) isEventRangeIndexed(ctx context.Context, tx *sql.Tx, coverage *eventRangeCoverage) (bool, error) {
	if len(coverage.tipsets) == 0 {
		return true, nil
	}

	rows, err := tx.Stmt(si.stmts.getTipsetEventCompletionsByHeightStmt).QueryContext(ctx, coverage.minHeight, coverage.maxHeight)
	if err != nil {
		return false, xerrors.Errorf("failed to query event index completion for range: %w", err)
	}
	defer func() { _ = rows.Close() }()

	completedTipsets := 0
	for rows.Next() {
		var tipsetKeyCidBytes []byte
		if err := rows.Scan(&tipsetKeyCidBytes); err != nil {
			return false, xerrors.Errorf("failed to read event index completion for range: %w", err)
		}
		tipsetKeyCid, err := cid.Cast(tipsetKeyCidBytes)
		if err != nil {
			return false, xerrors.Errorf("failed to parse event index completion tipset cid: %w", err)
		}
		if _, ok := coverage.tipsets[tipsetKeyCid]; ok {
			// tipset_bloom.tipset_key_cid is a primary key, so each expected
			// tipset can contribute at most once.
			completedTipsets++
		}
	}
	if err := rows.Err(); err != nil {
		return false, xerrors.Errorf("failed while reading event index completion for range: %w", err)
	}

	return completedTipsets == len(coverage.tipsets), nil
}

// GetEventsForFilter returns matching events for the given filter
// Returns nil, nil if the filter has no matching events
// Returns nil, ErrNotFound if event-index completion cannot be established for the requested tipsets
// Returns nil, ErrBackfillRequired if the index is in degraded mode and requires a backfill
// Returns nil, err for all other errors
func (si *SqliteIndexer) GetEventsForFilter(ctx context.Context, f *EventFilter) ([]*CollectedEvent, error) {
	if si.needsBackfill {
		return nil, ErrBackfillRequired
	}

	getEventsFnc := func(stmt *sql.Stmt, values []any) ([]*CollectedEvent, error) {
		q, err := stmt.QueryContext(ctx, values...)
		if err != nil {
			return nil, xerrors.Errorf("failed to query events: %w", err)
		}
		defer func() { _ = q.Close() }()

		var ces []*CollectedEvent
		var currentID int64 = -1
		var lastHeight abi.ChainEpoch = -1
		var tipsetsSeen int
		var ce *CollectedEvent

		// Rows are sorted by (height, message_index, event_index), so consecutive events
		// usually share tipset_key_cid and message_cid; cache the last seen to skip work.
		var lastTsKeyCid cid.Cid
		var lastTsKey types.TipSetKey
		var lastMsgCid cid.Cid

		// Memoize emitter address; the flag handles invalidation when the source path
		// switches between ID actor and delegated bytes.
		var (
			lastEmitterAddr      address.Address // address.Undef until first set
			lastEmitterIsID      bool            // true if last was derived from emitterID
			lastEmitterID        uint64          // valid when lastEmitterIsID
			lastEmitterAddrBytes string          // valid when !lastEmitterIsID && set
		)

		// Reused across rows; declaring inside the loop allocates fresh slots each iteration.
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

		for q.Next() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
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

			// The query returns all entries for all matching events; create a new CollectedEvent each time we see a new id.
			if row.id != currentID {
				rowHeight := abi.ChainEpoch(row.height)
				if rowHeight != lastHeight {
					tipsetsSeen++
					lastHeight = rowHeight
				}

				currentID = row.id
				ce = &CollectedEvent{
					EventIdx: row.eventIndex,
					Reverted: row.reverted,
					Height:   rowHeight,
					MsgIdx:   row.messageIndex,
				}
				ces = append(ces, ce)

				// MaxResults applies as a hard cap only once events span more than one tipset;
				// a single contributing tipset may exceed the cap. Single-tipset and
				// single-message queries naturally bypass this because tipsetsSeen stays at 1.
				if f.MaxResults > 0 && tipsetsSeen > 1 && len(ces) > f.MaxResults {
					return nil, ErrMaxResultsReached
				}

				if row.emitterAddr == nil {
					if !lastEmitterIsID || row.emitterID != lastEmitterID || lastEmitterAddr == address.Undef {
						lastEmitterAddr, err = address.NewIDAddress(row.emitterID)
						if err != nil {
							return nil, xerrors.Errorf("failed to parse emitter id: %w", err)
						}
						lastEmitterIsID = true
						lastEmitterID = row.emitterID
					}
				} else {
					if lastEmitterIsID || string(row.emitterAddr) != lastEmitterAddrBytes || lastEmitterAddr == address.Undef {
						lastEmitterAddr, err = address.NewFromBytes(row.emitterAddr)
						if err != nil {
							return nil, xerrors.Errorf("parse emitter addr: %w", err)
						}
						lastEmitterIsID = false
						lastEmitterAddrBytes = string(row.emitterAddr)
					}
				}
				ce.EmitterAddr = lastEmitterAddr

				if string(row.tipsetKeyCid) != lastTsKeyCid.KeyString() {
					lastTsKeyCid, err = cid.Cast(row.tipsetKeyCid)
					if err != nil {
						return nil, xerrors.Errorf("parse tipsetkey cid: %w", err)
					}
					ts, err := si.cs.GetTipSetByCid(ctx, lastTsKeyCid)
					if err != nil {
						return nil, xerrors.Errorf("get tipset by cid: %w", err)
					}
					if ts == nil {
						return nil, xerrors.Errorf("failed to get tipset from cid: tipset is nil for cid: %s", lastTsKeyCid)
					}
					lastTsKey = ts.Key()
				}
				ce.TipSetKey = lastTsKey

				if string(row.messageCid) != lastMsgCid.KeyString() {
					lastMsgCid, err = cid.Cast(row.messageCid)
					if err != nil {
						return nil, xerrors.Errorf("parse message cid: %w", err)
					}
				}
				ce.MsgCid = lastMsgCid
			}

			ce.Entries = append(ce.Entries, types.EventEntry{
				Flags: row.flags[0],
				Key:   row.key,
				Codec: row.codec,
				Value: row.value,
			})
		}
		if err := q.Err(); err != nil {
			return nil, xerrors.Errorf("failed while reading events: %w", err)
		}

		if len(ces) == 0 {
			return nil, nil
		}

		return ces, nil
	}

	queryFilter := f
	var (
		rangeCoverage *eventRangeCoverage
		err           error
	)
	if f.TipsetCid == cid.Undef {
		rangeCoverage, err = si.getEventRangeCoverage(ctx, f.MinHeight, f.MaxHeight)
		if err != nil {
			return nil, xerrors.Errorf("failed to determine canonical event range: %w", err)
		}
		normalized := *f
		normalized.MinHeight = rangeCoverage.minHeight
		normalized.MaxHeight = rangeCoverage.maxHeight
		queryFilter = &normalized
	}

	values, query, err := makePrefillFilterQuery(queryFilter)
	if err != nil {
		return nil, xerrors.Errorf("failed to make prefill filter query: %w", err)
	}

	stmt, err := si.db.Prepare(query)
	if err != nil {
		return nil, xerrors.Errorf("prepare prefill query: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	// indexEvents writes the tipset bloom after all event rows, including an
	// empty bloom for a tipset with no events. Read rows and completion markers
	// in one snapshot so a concurrent Apply, backfill, or GC cannot turn an
	// incomplete read into an authoritative empty or partial result.
	isTipsetCIDQuery := f.TipsetCid != cid.Undef
	getEvents := func() ([]*CollectedEvent, bool, error) {
		tx, err := si.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, false, xerrors.Errorf("failed to begin event index read transaction: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		if rangeCoverage != nil {
			complete, err := si.isEventRangeIndexed(ctx, tx, rangeCoverage)
			if err != nil {
				return nil, false, err
			}
			if !complete {
				return nil, false, nil
			}
		}

		ces, err := getEventsFnc(tx.Stmt(stmt), values)
		if err != nil {
			return nil, false, err
		}
		if rangeCoverage != nil {
			var lastTipsetKey types.TipSetKey
			for i, ce := range ces {
				// Results are ordered by height, message, and event index, so a
				// contributing tipset's events are contiguous. Validate each run
				// once instead of hashing the same TipSetKey for every event.
				if i > 0 && ce.TipSetKey == lastTipsetKey {
					continue
				}
				tsKeyCid, err := ce.TipSetKey.Cid()
				if err != nil {
					return nil, false, xerrors.Errorf("failed to get event tipset key cid: %w", err)
				}
				if _, ok := rangeCoverage.tipsets[tsKeyCid]; !ok {
					return nil, false, nil
				}
				lastTipsetKey = ce.TipSetKey
			}
			return ces, true, nil
		}

		if len(ces) > 0 {
			// Preserve reads from databases created before tipset blooms were
			// introduced. Event rows are themselves sufficient for a non-empty
			// result because indexEvents commits all rows atomically.
			return ces, true, nil
		}

		var complete bool
		if err := tx.Stmt(si.stmts.hasTipsetEventCompletionStmt).QueryRowContext(ctx, f.TipsetCid.Bytes()).Scan(&complete); err != nil {
			return nil, false, xerrors.Errorf("failed to check if tipset event indexing is complete: %w", err)
		}
		return nil, complete, nil
	}

	ces, eventsComplete, err := getEvents()
	if err != nil {
		return nil, xerrors.Errorf("failed to get events: %w", err)
	}
	if eventsComplete {
		return ces, nil
	}

	height := queryFilter.MaxHeight
	if isTipsetCIDQuery {
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
		if height <= head.Height()-maxLookBackForWait {
			return nil, ErrNotFound
		}
	}

	// Recent coverage may still be catching up. Preserve the existing wait,
	// then require the event-completion markers on the retry as well.
	if err := si.waitTillHeadIndexed(ctx); err != nil {
		return nil, xerrors.Errorf("failed to wait for head to be indexed: %w", err)
	}
	ces, eventsComplete, err = getEvents()
	if err != nil {
		return nil, xerrors.Errorf("failed to get events: %w", err)
	}
	if !eventsComplete {
		return nil, ErrNotFound
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
		clauses = append(clauses, "tm.reverted=?", "e.reverted=?")
		values = append(values, false, false)
	}

	if f.MsgCid != cid.Undef {
		clauses = append(clauses, "tm.message_cid=?")
		values = append(values, f.MsgCid.Bytes())
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
