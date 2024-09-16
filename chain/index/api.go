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
	if err != nil {
		return xerrors.Errorf("failed to get max non reverted height: %w", err)
	}
	if !maxNonRevertedHeight.Valid {
		return nil
	}
	if epoch >= abi.ChainEpoch(maxNonRevertedHeight.Int64) {
		return xerrors.Errorf("failed to validate index at epoch %d; can only validate at or before epoch %d for safety", epoch, maxNonRevertedHeight.Int64-1)
	}

	return nil
}

func (si *SqliteIndexer) validateNullRound(ctx context.Context, epoch abi.ChainEpoch) (*types.IndexValidation, error) {
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
	if !si.started && si.closed {
		return nil, xerrors.Errorf("ChainValidateIndex can only be called after the indexer has been started and not closed")
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
	expectedTsKeyCid, err := expectedTs.Key().Cid()
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset key cid: %w", err)
	}

	// Canonical chain has a null round at the epoch -> return if index is empty otherwise validate
	if expectedTs.Height() != epoch { // Canonical chain has a null round at the epoch
		if isIndexEmpty {
			return nil, nil
		}
		return si.validateNullRound(ctx, epoch)
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
	if revertedCount == 0 && nonRevertedCount == 0 {
		return si.backfillMissingTipset(ctx, expectedTs, backfill)
	}

	if revertedCount > 0 && nonRevertedCount == 0 {
		return nil, xerrors.Errorf("index corruption: height %d only has reverted tipsets", epoch)
	}

	if nonRevertedCount > 1 {
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

	if !indexedTsKeyCid.Equals(expectedTsKeyCid) {
		return nil, xerrors.Errorf("index corruption: non-reverted tipset at height %d has key %s, but canonical chain has %s", epoch, indexedTsKeyCid, expectedTsKeyCid)
	}

	indexedNonRevertedMsgCount, indexedNonRevertedEventsCount, hasRevertedEvents, err := si.getIndexedTipSetData(ctx, expectedTs.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset message and event counts at height %d: %w", expectedTs.Height(), err)
	}

	if err = si.verifyIndexedData(ctx, expectedTs, indexedNonRevertedMsgCount); err != nil {
		return nil, xerrors.Errorf("failed to verify indexed data at height %d: %w", expectedTs.Height(), err)
	}

	return &types.IndexValidation{
		TipsetKey:      expectedTs.Key().String(),
		Height:         uint64(expectedTs.Height()),
		TotalMessages:  indexedNonRevertedMsgCount,
		TotalEvents:    indexedNonRevertedEventsCount,
		EventsReverted: hasRevertedEvents,
	}, nil
}

// verifyIndexedData verifies that the indexed data for a tipset is correct
// by comparing the number of messages in the chainstore to the number of messages indexed

// TODO: verify indexed events too (to verify the events we need to load the next tipset (ts+1) and verify the events are the same)
func (si *SqliteIndexer) verifyIndexedData(ctx context.Context, ts *types.TipSet, indexedMsgCount uint64) (err error) {
	msgs, err := si.cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return xerrors.Errorf("failed to get messages for tipset: %w", err)
	}

	msgCount := uint64(len(msgs))
	if msgCount != indexedMsgCount {
		return xerrors.Errorf("tipset message count mismatch: chainstore has %d, index has %d", msgCount, indexedMsgCount)
	}

	return nil
}

func (si *SqliteIndexer) getIndexedTipSetData(ctx context.Context, tsKey types.TipSetKey) (messageCount uint64, eventCount uint64, hasRevertedEvents bool, err error) {
	tsKeyBytes := tsKey.Bytes()

	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		if err = tx.Stmt(si.stmts.getNonRevertedTipsetMessageCountStmt).QueryRowContext(ctx, tsKeyBytes).Scan(&messageCount); err != nil {
			return xerrors.Errorf("failed to query non reverted message count: %w", err)
		}

		if err = tx.Stmt(si.stmts.getNonRevertedTipsetEventCountStmt).QueryRowContext(ctx, tsKeyBytes).Scan(&eventCount); err != nil {
			return xerrors.Errorf("failed to query non reverted event count: %w", err)
		}

		if eventCount > 0 {
			if err = tx.Stmt(si.stmts.hasRevertedEventsStmt).QueryRowContext(ctx, tsKeyBytes).Scan(&hasRevertedEvents); err != nil {
				return xerrors.Errorf("failed to check for reverted events: %w", err)
			}
		}

		return nil
	})

	return messageCount, eventCount, hasRevertedEvents, err
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

	indexedNonRevertedMsgCount, indexedNonRevertedEventsCount, hasRevertedEvents, err := si.getIndexedTipSetData(ctx, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset message and event counts at height %d: %w", ts.Height(), err)
	}

	return &types.IndexValidation{
		TipsetKey:      ts.Key().String(),
		Height:         uint64(ts.Height()),
		Backfilled:     true,
		TotalMessages:  indexedNonRevertedMsgCount,
		TotalEvents:    indexedNonRevertedEventsCount,
		EventsReverted: hasRevertedEvents,
	}, nil
}
