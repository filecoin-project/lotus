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
	if !si.started {
		return nil, xerrors.Errorf("ChainValidateIndex can only be called after the indexer has been started")
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

	if indexedTsKeyCid != expectedTsKeyCid {
		return nil, xerrors.Errorf("index corruption: non-reverted tipset at height %d has key %s, but canonical chain has %s", epoch, indexedTsKeyCid, expectedTsKeyCid)
	}

	return &types.IndexValidation{
		TipsetKey: expectedTs.Key().String(),
		Height:    uint64(expectedTs.Height()),
		// TODO Other fields
	}, nil
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

	return &types.IndexValidation{
		TipsetKey:  ts.Key().String(),
		Height:     uint64(ts.Height()),
		Backfilled: true,
	}, nil
}
