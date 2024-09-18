package index

import (
	"context"
	"database/sql"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

func (si *SqliteIndexer) sanityCheckBackfillEpoch(ctx context.Context, epoch abi.ChainEpoch) error {
	// should be less than the max non reverted height in the Index
	var maxNonRevertedHeight sql.NullInt64
	err := si.stmts.getMaxNonRevertedHeightStmt.QueryRowContext(ctx).Scan(&maxNonRevertedHeight)
	if err != nil && err != sql.ErrNoRows {
		return xerrors.Errorf("failed to get max non reverted height: %w", err)
	}
	// couldn't find any non-reverted entries
	if err == sql.ErrNoRows || !maxNonRevertedHeight.Valid {
		return nil
	}
	if epoch >= abi.ChainEpoch(maxNonRevertedHeight.Int64) {
		return xerrors.Errorf("failed to validate index at epoch %d; can only validate at or before epoch %d for safety", epoch, maxNonRevertedHeight.Int64-1)
	}

	return nil
}

func (si *SqliteIndexer) validateIsNullRound(ctx context.Context, epoch abi.ChainEpoch) (*types.IndexValidation, error) {
	// make sure we do not have ANY non-reverted tipset at this epoch
	var isNullRound bool
	err := si.stmts.hasNullRoundAtHeightStmt.QueryRowContext(ctx, epoch).Scan(&isNullRound)
	if err != nil {
		return nil, xerrors.Errorf("failed to check if null round exists at height %d: %w", epoch, err)
	}
	if !isNullRound {
		return nil, xerrors.Errorf("index corruption: height %d should be a null round but is not", epoch)
	}

	return nil, nil
}

func (si *SqliteIndexer) getTipsetCountsAtHeight(ctx context.Context, height abi.ChainEpoch) (revertedCount, nonRevertedCount int, err error) {
	err = si.stmts.countTipsetsAtHeightStmt.QueryRowContext(ctx, height).Scan(&revertedCount, &nonRevertedCount)
	if err != nil {
		if err == sql.ErrNoRows {
			// No tipsets found at this height
			return 0, 0, nil
		}
		return 0, 0, xerrors.Errorf("failed to query tipset counts: %w", err)
	}

	return revertedCount, nonRevertedCount, nil
}

func (si *SqliteIndexer) ChainValidateIndex(ctx context.Context, epoch abi.ChainEpoch, backfill bool) (*types.IndexValidation, error) {
	if !si.started {
		return nil, xerrors.Errorf("ChainValidateIndex can only be called after the indexer has been started")
	}

	if si.isClosed() {
		return nil, xerrors.Errorf("ChainValidateIndex can only be called before the indexer has been closed")
	}

	si.writerLk.Lock()
	defer si.writerLk.Unlock()

	if err := si.sanityCheckBackfillEpoch(ctx, epoch); err != nil {
		return nil, err
	}

	var isIndexEmpty bool
	err := si.stmts.isIndexEmptyStmt.QueryRowContext(ctx).Scan(&isIndexEmpty)
	if err != nil {
		return nil, xerrors.Errorf("failed to check if index is empty: %w", err)
	}

	// fetch the tipset at the given epoch on the canonical chain
	expectedTs, err := si.cs.GetTipsetByHeight(ctx, epoch, nil, true)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset by height: %w", err)
	}

	// Canonical chain has a null round at the epoch -> return if index is empty otherwise validate
	if expectedTs.Height() != epoch { // Canonical chain has a null round at the epoch
		if isIndexEmpty {
			return nil, nil
		}
		// validate the db has a hole here and error if not, we don't attempt to repair because something must be very wrong for this to fail
		return si.validateIsNullRound(ctx, epoch)
	}

	// if the index is empty -> short-circuit and simply backfill if applicable
	if isIndexEmpty {
		return si.backfillMissingTipset(ctx, expectedTs, backfill)
	}

	revertedCount, nonRevertedCount, err := si.getTipsetCountsAtHeight(ctx, epoch)
	if err != nil {
		if err == sql.ErrNoRows {
			return si.backfillMissingTipset(ctx, expectedTs, backfill)
		}
		return nil, xerrors.Errorf("failed to get tipset counts at height: %w", err)
	}
	switch {
	case revertedCount == 0 && nonRevertedCount == 0:
		return si.backfillMissingTipset(ctx, expectedTs, backfill)

	case revertedCount > 0 && nonRevertedCount == 0:
		return nil, xerrors.Errorf("index corruption: height %d only has reverted tipsets", epoch)

	case nonRevertedCount > 1:
		return nil, xerrors.Errorf("index corruption: height %d has multiple non-reverted tipsets", epoch)
	}

	// fetch the non-reverted tipset at this epoch
	var indexedTsKeyCidBytes []byte
	err = si.stmts.getNonRevertedTipsetAtHeightStmt.QueryRowContext(ctx, epoch).Scan(&indexedTsKeyCidBytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to get non-reverted tipset at height: %w", err)
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
		return nil, xerrors.Errorf("index corruption: non-reverted tipset at height %d has key %s, but canonical chain has %s", epoch, indexedTsKeyCid, expectedTsKeyCid)
	}

	// indexedTsKeyCid and expectedTsKeyCid are the same, so we can use `expectedTs` to fetch the indexed data
	indexedData, err := si.getIndexedTipSetData(ctx, expectedTs)
	if err != nil {
		return nil, xerrors.Errorf("failed to get indexed data for tipset at height %d: %w", expectedTs.Height(), err)
	}

	if indexedData == nil {
		return nil, xerrors.Errorf("invalid indexed data for tipset at height %d", expectedTs.Height())
	}

	if err = si.verifyIndexedData(ctx, expectedTs, indexedData); err != nil {
		return nil, xerrors.Errorf("failed to verify indexed data at height %d: %w", expectedTs.Height(), err)
	}

	return &types.IndexValidation{
		TipSetKey:            expectedTs.Key(),
		Height:               uint64(expectedTs.Height()),
		IndexedMessagesCount: uint64(indexedData.nonRevertedMessageCount),
		IndexedEventsCount:   uint64(indexedData.nonRevertedEventCount),
	}, nil
}

type indexedTipSetData struct {
	nonRevertedMessageCount int
	nonRevertedEventCount   int
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

		return nil
	})

	return &data, err
}

// verifyIndexedData verifies that the indexed data for a tipset is correct
// by comparing the number of messages and events in the chainstore to the number of messages and events indexed.
// NOTE: Events are loaded from the executed messages of the tipset at the next epoch (ts.Height() + 1).
func (si *SqliteIndexer) verifyIndexedData(ctx context.Context, ts *types.TipSet, indexedData *indexedTipSetData) (err error) {
	tsKeyCid, err := ts.Key().Cid()
	if err != nil {
		return xerrors.Errorf("failed to get tipset key cid: %w", err)
	}

	// get the tipset where the messages of `ts` will be executed (deferred execution)
	executionTs, err := si.cs.GetTipsetByHeight(ctx, ts.Height()+1, nil, false)
	if err != nil {
		return xerrors.Errorf("failed to get tipset by height: %w", err)
	}

	eParentTsKeyCid, err := executionTs.Parents().Cid()
	if err != nil {
		return xerrors.Errorf("failed to get execution tipset parent key cid: %w", err)
	}

	// the parent tipset of the execution tipset should be the same as the indexed tipset (`ts` should be the parent of `executionTs`)
	// if that is not the case, it means that the chain forked after we fetched the tipset `ts` from the canonical chain and
	// `ts` is no longer part of the canonical chain. Simply return an error here and ask the user to retry.
	if !eParentTsKeyCid.Equals(tsKeyCid) {
		return xerrors.Errorf("execution tipset parent key mismatch: chainstore has %s, index has %s; please retry your request as this could have been caused by a chain reorg",
			eParentTsKeyCid, tsKeyCid)
	}

	executedMsgs, err := si.loadExecutedMessages(ctx, ts, executionTs)
	if err != nil {
		return xerrors.Errorf("failed to load executed messages: %w", err)
	}

	totalEventsCount := 0
	for _, emsg := range executedMsgs {
		totalEventsCount += len(emsg.evs)
	}

	if totalEventsCount != indexedData.nonRevertedEventCount {
		return xerrors.Errorf("event count mismatch: chainstore has %d, index has %d", totalEventsCount, indexedData.nonRevertedEventCount)
	}

	totalExecutedMsgCount := len(executedMsgs)
	if totalExecutedMsgCount != indexedData.nonRevertedMessageCount {
		return xerrors.Errorf("message count mismatch: chainstore has %d, index has %d", totalExecutedMsgCount, indexedData.nonRevertedMessageCount)
	}

	// if non-reverted events exist which means that tipset `ts` has been executed, there should be 0 reverted events in the DB
	var hasRevertedEventsInTipset bool
	err = si.stmts.hasRevertedEventsInTipsetStmt.QueryRowContext(ctx, tsKeyCid.Bytes()).Scan(&hasRevertedEventsInTipset)
	if err != nil {
		return xerrors.Errorf("failed to check if there are reverted events in tipset: %w", err)
	}
	if hasRevertedEventsInTipset {
		return xerrors.Errorf("index corruption: reverted events found for an executed tipset %s", tsKeyCid)
	}

	return nil
}

func (si *SqliteIndexer) backfillMissingTipset(ctx context.Context, ts *types.TipSet, backfill bool) (*types.IndexValidation, error) {
	if !backfill {
		return nil, xerrors.Errorf("missing tipset at height %d in the chain index, set backfill to true to backfill", ts.Height())
	}
	// backfill the tipset in the Index
	parentTs, err := si.cs.GetTipSetFromKey(ctx, ts.Parents())
	if err != nil {
		return nil, xerrors.Errorf("failed to get parent tipset: %w", err)
	}

	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		if err := si.indexTipsetWithParentEvents(ctx, tx, parentTs, ts); err != nil {
			return xerrors.Errorf("error indexing tipset: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, xerrors.Errorf("error applying tipset: %w", err)
	}

	indexedData, err := si.getIndexedTipSetData(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset message and event counts at height %d: %w", ts.Height(), err)
	}

	return &types.IndexValidation{
		TipSetKey:            ts.Key(),
		Height:               uint64(ts.Height()),
		Backfilled:           true,
		IndexedMessagesCount: uint64(indexedData.nonRevertedMessageCount),
		IndexedEventsCount:   uint64(indexedData.nonRevertedEventCount),
	}, nil
}
