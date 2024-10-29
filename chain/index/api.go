package index

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
)

var ErrChainForked = xerrors.New("chain forked")

func (si *SqliteIndexer) ChainValidateIndex(ctx context.Context, epoch abi.ChainEpoch, backfill bool) (*types.IndexValidation, error) {
	// return an error if the indexer is not started
	if !si.started {
		return nil, errors.New("ChainValidateIndex called before indexer start")
	}

	// return an error if the indexer is closed
	if si.isClosed() {
		return nil, errors.New("ChainValidateIndex called on closed indexer")
	}

	// this API only works for epoch < head because of deferred execution in Filecoin
	head := si.cs.GetHeaviestTipSet()
	if epoch >= head.Height() {
		return nil, xerrors.Errorf("cannot validate index at epoch %d, can only validate at an epoch less than chain head epoch %d", epoch, head.Height())
	}

	// fetch the tipset at the given epoch on the canonical chain
	expectedTs, err := si.cs.GetTipsetByHeight(ctx, epoch, head, true)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset at height %d: %w", epoch, err)
	}

	// we need to take a write lock here so that back-filling does not race with real-time chain indexing
	if backfill {
		si.writerLk.Lock()
		defer si.writerLk.Unlock()
	}

	var isIndexEmpty bool
	if err := si.stmts.isIndexEmptyStmt.QueryRowContext(ctx).Scan(&isIndexEmpty); err != nil {
		return nil, xerrors.Errorf("failed to check if index is empty: %w", err)
	}

	// Canonical chain has a null round at the epoch -> return if index is empty otherwise validate that index also
	// has a null round at this epoch i.e. it does not have anything indexed at all for this epoch
	if expectedTs.Height() != epoch {
		if isIndexEmpty {
			return &types.IndexValidation{
				Height:      epoch,
				IsNullRound: true,
			}, nil
		}
		// validate the db has a hole here and error if not, we don't attempt to repair because something must be very wrong for this to fail
		return si.validateIsNullRound(ctx, epoch)
	}

	// if the index is empty -> short-circuit and simply backfill if applicable
	if isIndexEmpty {
		if !backfill {
			return nil, makeBackfillRequiredErr(epoch)
		}
		return si.backfillMissingTipset(ctx, expectedTs)
	}
	// see if the tipset at this epoch is already indexed or if we need to backfill
	revertedCount, nonRevertedCount, err := si.getTipsetCountsAtHeight(ctx, epoch)
	if err != nil {
		if err == sql.ErrNoRows {
			if !backfill {
				return nil, makeBackfillRequiredErr(epoch)
			}
			return si.backfillMissingTipset(ctx, expectedTs)
		}
		return nil, xerrors.Errorf("failed to get tipset counts at height %d: %w", epoch, err)
	}

	switch {
	case revertedCount == 0 && nonRevertedCount == 0:
		// no tipsets at this epoch in the index, backfill
		if !backfill {
			return nil, makeBackfillRequiredErr(epoch)
		}
		return si.backfillMissingTipset(ctx, expectedTs)

	case revertedCount > 0 && nonRevertedCount == 0:
		return nil, xerrors.Errorf("index corruption: height %d only has reverted tipsets", epoch)

	case nonRevertedCount > 1:
		return nil, xerrors.Errorf("index corruption: height %d has multiple non-reverted tipsets", epoch)
	}

	// fetch the non-reverted tipset at this epoch
	var indexedTsKeyCidBytes []byte
	err = si.stmts.getNonRevertedTipsetAtHeightStmt.QueryRowContext(ctx, epoch).Scan(&indexedTsKeyCidBytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to get non-reverted tipset at height %d: %w", epoch, err)
	}

	indexedTsKeyCid, err := cid.Cast(indexedTsKeyCidBytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to cast tipset key cid: %w", err)
	}
	expectedTsKeyCid, err := expectedTs.Key().Cid()
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset key cid: %w", err)
	}
	if !indexedTsKeyCid.Equals(expectedTsKeyCid) {
		return nil, xerrors.Errorf("index corruption: indexed tipset at height %d has key %s, but canonical chain has %s", epoch, indexedTsKeyCid, expectedTsKeyCid)
	}

	getAndVerifyIndexedData := func() (*indexedTipSetData, error) {
		indexedData, err := si.getIndexedTipSetData(ctx, expectedTs)
		if err != nil {
			return nil, xerrors.Errorf("failed to get indexed data for tipset at height %d: %w", expectedTs.Height(), err)
		}
		if indexedData == nil {
			return nil, xerrors.Errorf("nil indexed data for tipset at height %d", expectedTs.Height())
		}
		if err = si.verifyIndexedData(ctx, expectedTs, indexedData); err != nil {
			return nil, err
		}
		return indexedData, nil
	}

	indexedData, err := getAndVerifyIndexedData()
	var bf bool
	if err != nil {
		if !backfill {
			return nil, xerrors.Errorf("failed to verify indexed data at height %d: %w", expectedTs.Height(), err)
		}

		log.Warnf("failed to verify indexed data at height %d; err:%s; backfilling once and validating again", expectedTs.Height(), err)
		if _, err := si.backfillMissingTipset(ctx, expectedTs); err != nil {
			return nil, xerrors.Errorf("failed to backfill missing tipset at height %d during validation; err: %w", expectedTs.Height(), err)
		}

		indexedData, err = getAndVerifyIndexedData()
		if err != nil {
			return nil, xerrors.Errorf("failed to verify indexed data at height %d after backfill: %w", expectedTs.Height(), err)
		}
		bf = true
	}

	return &types.IndexValidation{
		TipSetKey:                expectedTs.Key(),
		Height:                   expectedTs.Height(),
		IndexedMessagesCount:     indexedData.nonRevertedMessageCount,
		IndexedEventsCount:       indexedData.nonRevertedEventCount,
		IndexedEventEntriesCount: indexedData.nonRevertedEventEntriesCount,
		Backfilled:               bf,
	}, nil
}

func (si *SqliteIndexer) validateIsNullRound(ctx context.Context, epoch abi.ChainEpoch) (*types.IndexValidation, error) {
	// make sure we do not have tipset(reverted or non-reverted) indexed at this epoch
	var isNullRound bool
	err := si.stmts.hasNullRoundAtHeightStmt.QueryRowContext(ctx, epoch).Scan(&isNullRound)
	if err != nil {
		return nil, xerrors.Errorf("failed to check if null round exists at height %d: %w", epoch, err)
	}
	if !isNullRound {
		return nil, xerrors.Errorf("index corruption: height %d should be a null round but is not", epoch)
	}

	return &types.IndexValidation{
		Height:      epoch,
		IsNullRound: true,
	}, nil
}

func (si *SqliteIndexer) getTipsetCountsAtHeight(ctx context.Context, height abi.ChainEpoch) (revertedCount, nonRevertedCount int, err error) {
	err = si.stmts.countTipsetsAtHeightStmt.QueryRowContext(ctx, height).Scan(&revertedCount, &nonRevertedCount)
	if err != nil {
		if err == sql.ErrNoRows {
			// No tipsets found at this height
			return 0, 0, nil
		}
		return 0, 0, xerrors.Errorf("failed to query tipset counts at height %d: %w", height, err)
	}

	return revertedCount, nonRevertedCount, nil
}

type indexedTipSetData struct {
	nonRevertedMessageCount      uint64
	nonRevertedEventCount        uint64
	nonRevertedEventEntriesCount uint64
}

// getIndexedTipSetData fetches the indexed tipset data for a tipset
func (si *SqliteIndexer) getIndexedTipSetData(ctx context.Context, ts *types.TipSet) (*indexedTipSetData, error) {
	tsKeyCidBytes, err := toTipsetKeyCidBytes(ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset key cid: %w", err)
	}

	var data indexedTipSetData
	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		if err = tx.Stmt(si.stmts.getNonRevertedTipsetMessageCountStmt).QueryRowContext(ctx, tsKeyCidBytes).Scan(&data.nonRevertedMessageCount); err != nil {
			return xerrors.Errorf("failed to query non reverted message count: %w", err)
		}

		if err = tx.Stmt(si.stmts.getNonRevertedTipsetEventCountStmt).QueryRowContext(ctx, tsKeyCidBytes).Scan(&data.nonRevertedEventCount); err != nil {
			return xerrors.Errorf("failed to query non reverted event count: %w", err)
		}

		if err = tx.Stmt(si.stmts.getNonRevertedTipsetEventEntriesCountStmt).QueryRowContext(ctx, tsKeyCidBytes).Scan(&data.nonRevertedEventEntriesCount); err != nil {
			return xerrors.Errorf("failed to query non reverted event entries count: %w", err)
		}

		return nil
	})

	return &data, err
}

// verifyIndexedData verifies that the indexed data for a tipset is correct
// by comparing the number of messages and events in the chainstore to the number of messages and events indexed.
//
// Notes:
//
//   - Events are loaded from the executed messages of the tipset at the next epoch (ts.Height() + 1).
//   - This is not a comprehensive verification because we only compare counts, assuming that a match
//     means that the entries are correct. A future iteration may compare message and event details to
//     confirm that they are what is expected.
func (si *SqliteIndexer) verifyIndexedData(ctx context.Context, ts *types.TipSet, indexedData *indexedTipSetData) (err error) {
	tsKeyCid, err := ts.Key().Cid()
	if err != nil {
		return xerrors.Errorf("failed to get tipset key cid at height %d: %w", ts.Height(), err)
	}

	executionTs, err := si.getNextTipset(ctx, ts)
	if err != nil {
		return xerrors.Errorf("failed to get next tipset for height %d: %w", ts.Height(), err)
	}

	// given that `ts` is on the canonical chain and `executionTs` is the next tipset in the chain
	// `ts` can not have reverted events
	var hasRevertedEventsInTipset bool
	err = si.stmts.hasRevertedEventsInTipsetStmt.QueryRowContext(ctx, tsKeyCid.Bytes()).Scan(&hasRevertedEventsInTipset)
	if err != nil {
		return xerrors.Errorf("failed to check if there are reverted events in tipset for height %d: %w", ts.Height(), err)
	}
	if hasRevertedEventsInTipset {
		return xerrors.Errorf("index corruption: reverted events found for an executed tipset %s at height %d", tsKeyCid, ts.Height())
	}

	executedMsgs, err := si.executedMessagesLoaderFunc(ctx, si.cs, ts, executionTs)
	if err != nil {
		return xerrors.Errorf("failed to load executed messages for height %d: %w", ts.Height(), err)
	}

	var (
		totalEventsCount       = uint64(0)
		totalEventEntriesCount = uint64(0)
	)
	for _, emsg := range executedMsgs {
		totalEventsCount += uint64(len(emsg.evs))
		for _, ev := range emsg.evs {
			totalEventEntriesCount += uint64(len(ev.Entries))
		}
	}

	if totalEventsCount != indexedData.nonRevertedEventCount {
		return xerrors.Errorf("event count mismatch for height %d: chainstore has %d, index has %d", ts.Height(), totalEventsCount, indexedData.nonRevertedEventCount)
	}

	totalExecutedMsgCount := uint64(len(executedMsgs))
	if totalExecutedMsgCount != indexedData.nonRevertedMessageCount {
		return xerrors.Errorf("message count mismatch for height %d: chainstore has %d, index has %d", ts.Height(), totalExecutedMsgCount, indexedData.nonRevertedMessageCount)
	}

	if indexedData.nonRevertedEventEntriesCount != totalEventEntriesCount {
		return xerrors.Errorf("event entries count mismatch for height %d: chainstore has %d, index has %d", ts.Height(), totalEventEntriesCount, indexedData.nonRevertedEventEntriesCount)
	}

	// compare the events AMT root between the indexed events and the events in the chain state
	for _, emsg := range executedMsgs {
		indexedRoot, hasEvents, err := si.amtRootForEvents(ctx, tsKeyCid, emsg.msg.Cid())
		if err != nil {
			return xerrors.Errorf("failed to generate AMT root for indexed events of message %s at height %d: %w", emsg.msg.Cid(), ts.Height(), err)
		}

		if !hasEvents && emsg.rct.EventsRoot == nil {
			// No events in index and no events in receipt, this is fine
			continue
		}

		if hasEvents && emsg.rct.EventsRoot == nil {
			return xerrors.Errorf("index corruption: events found in index for message %s at height %d, but message receipt has no events root", emsg.msg.Cid(), ts.Height())
		}

		if !hasEvents && emsg.rct.EventsRoot != nil {
			return xerrors.Errorf("index corruption: no events found in index for message %s at height %d, but message receipt has events root %s", emsg.msg.Cid(), ts.Height(), emsg.rct.EventsRoot)
		}

		// Both index and receipt have events, compare the roots
		if !indexedRoot.Equals(*emsg.rct.EventsRoot) {
			return xerrors.Errorf("index corruption: events AMT root mismatch for message %s at height %d. Index root: %s, Receipt root: %s", emsg.msg.Cid(), ts.Height(), indexedRoot, emsg.rct.EventsRoot)
		}
	}

	return nil
}

func (si *SqliteIndexer) backfillMissingTipset(ctx context.Context, ts *types.TipSet) (*types.IndexValidation, error) {
	executionTs, err := si.getNextTipset(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get next tipset at height %d: %w", ts.Height(), err)
	}

	backfillFunc := func() error {
		return withTx(ctx, si.db, func(tx *sql.Tx) error {
			return si.indexTipsetWithParentEvents(ctx, tx, ts, executionTs)
		})
	}

	if err := backfillFunc(); err != nil {
		if ipld.IsNotFound(err) {
			return nil, xerrors.Errorf("failed to backfill tipset at epoch %d: chain store does not contain data: %w", ts.Height(), err)
		}
		if ctx.Err() != nil {
			log.Errorf("failed to backfill tipset at epoch %d due to context cancellation: %s", ts.Height(), err)
		}
		return nil, xerrors.Errorf("failed to backfill tipset at epoch %d; err: %w", ts.Height(), err)
	}

	indexedData, err := si.getIndexedTipSetData(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get indexed tipset data: %w", err)
	}

	return &types.IndexValidation{
		TipSetKey:                ts.Key(),
		Height:                   ts.Height(),
		Backfilled:               true,
		IndexedMessagesCount:     indexedData.nonRevertedMessageCount,
		IndexedEventsCount:       indexedData.nonRevertedEventCount,
		IndexedEventEntriesCount: indexedData.nonRevertedEventEntriesCount,
	}, nil
}

func (si *SqliteIndexer) getNextTipset(ctx context.Context, ts *types.TipSet) (*types.TipSet, error) {
	nextEpochTs, err := si.cs.GetTipsetByHeight(ctx, ts.Height()+1, nil, false)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset at height %d: %w", ts.Height()+1, err)
	}

	if nextEpochTs.Parents() != ts.Key() {
		return nil, xerrors.Errorf("chain forked at height %d; please retry your request; err: %w", ts.Height(), ErrChainForked)
	}

	return nextEpochTs, nil
}

func makeBackfillRequiredErr(height abi.ChainEpoch) error {
	return xerrors.Errorf("missing tipset at height %d in the chain index, set backfill flag to true to fix", height)
}

// amtRootForEvents generates the events AMT root CID for a given message's events, and returns
// whether the message has events and a fatal error if one occurred.
func (si *SqliteIndexer) amtRootForEvents(
	ctx context.Context,
	tsKeyCid cid.Cid,
	msgCid cid.Cid,
) (cid.Cid, bool, error) {
	events := make([]cbg.CBORMarshaler, 0)

	err := withTx(ctx, si.db, func(tx *sql.Tx) error {
		rows, err := tx.Stmt(si.stmts.getEventIdAndEmitterIdStmt).QueryContext(ctx, tsKeyCid.Bytes(), msgCid.Bytes())
		if err != nil {
			return xerrors.Errorf("failed to query events: %w", err)
		}
		defer func() {
			_ = rows.Close()
		}()

		for rows.Next() {
			var eventId int
			var actorId int64
			if err := rows.Scan(&eventId, &actorId); err != nil {
				return xerrors.Errorf("failed to scan row: %w", err)
			}

			event := types.Event{
				Emitter: abi.ActorID(actorId),
				Entries: make([]types.EventEntry, 0),
			}

			rows2, err := tx.Stmt(si.stmts.getEventEntriesStmt).QueryContext(ctx, eventId)
			if err != nil {
				return xerrors.Errorf("failed to query event entries: %w", err)
			}
			defer func() {
				_ = rows2.Close()
			}()

			for rows2.Next() {
				var flags []byte
				var key string
				var codec uint64
				var value []byte
				if err := rows2.Scan(&flags, &key, &codec, &value); err != nil {
					return xerrors.Errorf("failed to scan row: %w", err)
				}
				entry := types.EventEntry{
					Flags: flags[0],
					Key:   key,
					Codec: codec,
					Value: value,
				}
				event.Entries = append(event.Entries, entry)
			}

			events = append(events, &event)
		}

		return nil
	})

	if err != nil {
		return cid.Undef, false, xerrors.Errorf("failed to retrieve events for message %s in tipset %s: %w", msgCid, tsKeyCid, err)
	}

	// construct the AMT from our slice to an in-memory IPLD store just so we can get the root,
	// we don't need the blocks themselves
	root, err := amt4.FromArray(ctx, cbor.NewCborStore(bstore.NewMemory()), events, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
	if err != nil {
		return cid.Undef, false, xerrors.Errorf("failed to create AMT: %w", err)
	}
	return root, len(events) > 0, nil
}
