package index

import (
	"context"
	"database/sql"

	ipld "github.com/ipfs/go-ipld-format"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

// ReconcileWithChain ensures that the index is consistent with the current chain state.
// It performs the following steps:
//  1. Checks if the index is empty. If so, it returns immediately as there's nothing to reconcile.
//  2. Finds the lowest non-reverted height in the index.
//  3. Walks backwards from the current chain head until it finds a tipset that exists
//     in the index and is not marked as reverted.
//  4. Sets a boundary epoch just above this found tipset.
//  5. Marks all tipsets above this boundary as reverted, ensuring consistency with the current chain state.
//  6. Applies all missing un-indexed tipsets starting from the last matching tipset b/w index and canonical chain
//     to the current chain head.
//
// This function is crucial for maintaining index integrity, especially after chain reorgs.
// It ensures that the index accurately reflects the current state of the blockchain.
func (si *SqliteIndexer) ReconcileWithChain(ctx context.Context, head *types.TipSet) error {
	if !si.cs.IsStoringEvents() {
		log.Warn("chain indexer is not storing events during reconciliation; please ensure this is intentional")
	}

	if si.isClosed() {
		return ErrClosed
	}

	if head == nil {
		return nil
	}

	return withTx(ctx, si.db, func(tx *sql.Tx) error {
		var isIndexEmpty bool
		err := tx.StmtContext(ctx, si.stmts.isIndexEmptyStmt).QueryRowContext(ctx).Scan(&isIndexEmpty)
		if err != nil {
			return xerrors.Errorf("failed to check if index is empty: %w", err)
		}

		if isIndexEmpty && !si.reconcileEmptyIndex {
			log.Info("chain index is empty and reconcileEmptyIndex is disabled; skipping reconciliation")
			return nil
		}

		if isIndexEmpty {
			log.Info("chain index is empty; backfilling from head")
			return si.backfillIndex(ctx, tx, head, 0)
		}

		reconciliationEpoch, err := si.getReconciliationEpoch(ctx, tx)
		if err != nil {
			return xerrors.Errorf("failed to get reconciliation epoch: %w", err)
		}

		currTs := head

		log.Infof("starting chain reconciliation from head height %d; reconciliation epoch is %d", head.Height(), reconciliationEpoch)

		// The goal here is to walk the canonical chain backwards from head until we find a matching non-reverted tipset
		// in the db so we know where to start reconciliation from
		// All tipsets that exist in the DB but not in the canonical chain are then marked as reverted
		// All tipsets that exist in the canonical chain but not in the db are then applied

		// we only need to walk back as far as the reconciliation epoch as all the tipsets in the index
		// below the reconciliation epoch are already marked as reverted because the reconciliation epoch
		// is the minimum non-reverted height in the index
		for currTs != nil && currTs.Height() >= reconciliationEpoch {
			tsKeyCidBytes, err := toTipsetKeyCidBytes(currTs)
			if err != nil {
				return xerrors.Errorf("failed to compute tipset cid: %w", err)
			}

			var exists bool
			err = tx.StmtContext(ctx, si.stmts.hasNonRevertedTipsetStmt).QueryRowContext(ctx, tsKeyCidBytes).Scan(&exists)
			if err != nil {
				return xerrors.Errorf("failed to check if tipset exists and is not reverted: %w", err)
			}

			if exists {
				// found it!
				reconciliationEpoch = currTs.Height() + 1
				log.Infof("found matching tipset at height %d, setting reconciliation epoch to %d", currTs.Height(), reconciliationEpoch)
				break
			}

			if currTs.Height() == 0 {
				log.Infof("ReconcileWithChain reached genesis but no matching tipset found in index")
				break
			}

			parents := currTs.Parents()
			currTs, err = si.cs.GetTipSetFromKey(ctx, parents)
			if err != nil {
				return xerrors.Errorf("failed to walk chain: %w", err)
			}
		}

		if currTs.Height() == 0 {
			log.Warn("ReconcileWithChain reached genesis without finding matching tipset")
		}

		// mark all tipsets from the reconciliation epoch onwards in the Index as reverted as they are not in the current canonical chain
		log.Infof("Marking tipsets as reverted from height %d", reconciliationEpoch)
		result, err := tx.StmtContext(ctx, si.stmts.updateTipsetsToRevertedFromHeightStmt).ExecContext(ctx, int64(reconciliationEpoch))
		if err != nil {
			return xerrors.Errorf("failed to mark tipsets as reverted: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return xerrors.Errorf("failed to get number of rows affected: %w", err)
		}

		// also need to mark events as reverted for the corresponding inclusion tipsets
		if _, err = tx.StmtContext(ctx, si.stmts.updateEventsToRevertedFromHeightStmt).ExecContext(ctx, int64(reconciliationEpoch-1)); err != nil {
			return xerrors.Errorf("failed to mark events as reverted: %w", err)
		}

		log.Infof("marked %d tipsets as reverted from height %d", rowsAffected, reconciliationEpoch)

		// if the head is less than the reconciliation epoch, we don't need to index any tipsets as we're already caught up
		if head.Height() < reconciliationEpoch {
			log.Info("no missing tipsets to index; index is already caught up with chain")
			return nil
		}

		// apply all missing tipsets by walking the chain backwards starting from head upto the reconciliation epoch
		log.Infof("indexing missing tipsets backwards from head height %d to reconciliation epoch %d", head.Height(), reconciliationEpoch)

		// if head.Height == reconciliationEpoch, this will only index head and return
		if err := si.backfillIndex(ctx, tx, head, reconciliationEpoch); err != nil {
			return xerrors.Errorf("failed to backfill index: %w", err)
		}

		return nil
	})
}

func (si *SqliteIndexer) getReconciliationEpoch(ctx context.Context, tx *sql.Tx) (abi.ChainEpoch, error) {
	var reconciliationEpochInIndex sql.NullInt64

	err := tx.StmtContext(ctx, si.stmts.getMinNonRevertedHeightStmt).
		QueryRowContext(ctx).
		Scan(&reconciliationEpochInIndex)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Warn("index only contains reverted tipsets; setting reconciliation epoch to 0")
			return 0, nil
		}
		return 0, xerrors.Errorf("failed to scan minimum non-reverted height: %w", err)
	}

	if !reconciliationEpochInIndex.Valid {
		log.Warn("index only contains reverted tipsets; setting reconciliation epoch to 0")
		return 0, nil
	}

	return abi.ChainEpoch(reconciliationEpochInIndex.Int64), nil
}

// backfillIndex backfills the chain index with missing tipsets starting from the given head tipset
// and stopping after the specified stopAfter epoch (inclusive).
//
// The behavior of this function depends on the relationship between head.Height and stopAfter:
//
// 1. If head.Height > stopAfter:
//   - The function will apply missing tipsets from head.Height down to stopAfter (inclusive).
//   - It will stop applying tipsets if the maximum number of tipsets to apply (si.maxReconcileTipsets) is reached.
//   - If the chain store only contains data up to a certain height, the function will stop backfilling at that height.
//
// 2. If head.Height == stopAfter:
//   - The function will only apply the head tipset and then return.
//
// 3. If head.Height < stopAfter:
//   - The function will immediately return without applying any tipsets.
//
// The si.maxReconcileTipsets parameter is used to limit the maximum number of tipsets that can be applied during the backfill process.
// If the number of applied tipsets reaches si.maxReconcileTipsets, the function will stop backfilling and return.
//
// The function also logs progress information at regular intervals (every builtin.EpochsInDay) to provide visibility into the backfill process.
func (si *SqliteIndexer) backfillIndex(ctx context.Context, tx *sql.Tx, head *types.TipSet, stopAfter abi.ChainEpoch) error {
	if head.Height() < stopAfter {
		return nil
	}

	currTs := head
	totalApplied := uint64(0)
	lastLoggedEpoch := head.Height()

	log.Infof("backfilling chain index backwards starting from head height %d", head.Height())

	// Calculate the actual number of tipsets to apply
	totalTipsetsToApply := min(uint64(head.Height()-stopAfter+1), si.maxReconcileTipsets)

	for currTs != nil {
		if totalApplied >= si.maxReconcileTipsets {
			log.Infof("reached maximum number of tipsets to apply (%d), finishing backfill; backfill applied %d tipsets",
				si.maxReconcileTipsets, totalApplied)
			return nil
		}

		err := si.applyMissingTipset(ctx, tx, currTs)
		if err != nil {
			if ipld.IsNotFound(err) {
				log.Infof("stopping backfill at height %d as chain store only contains data up to this height as per error %s; backfill applied %d tipsets",
					currTs.Height(), err, totalApplied)
				return nil
			}

			return xerrors.Errorf("failed to apply tipset at height %d: %w", currTs.Height(), err)
		}

		totalApplied++

		if lastLoggedEpoch-currTs.Height() >= builtin.EpochsInDay {
			progress := float64(totalApplied) / float64(totalTipsetsToApply) * 100
			log.Infof("backfill progress: %.2f%% complete (%d out of %d tipsets applied), ongoing", progress, totalApplied, totalTipsetsToApply)
			lastLoggedEpoch = currTs.Height()
		}

		if currTs.Height() == 0 {
			log.Infof("reached genesis tipset and have backfilled everything up to genesis; backfilled %d tipsets", totalApplied)
			return nil
		}

		if currTs.Height() <= stopAfter {
			log.Infof("reached stop height %d; backfilled %d tipsets", stopAfter, totalApplied)
			return nil
		}
		height := currTs.Height()
		currTs, err = si.cs.GetTipSetFromKey(ctx, currTs.Parents())
		if err != nil {
			return xerrors.Errorf("failed to walk chain beyond height %d: %w", height, err)
		}
	}

	log.Infof("applied %d tipsets during backfill", totalApplied)
	return nil
}

// applyMissingTipset indexes a single missing tipset and its parent events
// It's a simplified version of applyMissingTipsets, handling one tipset at a time
func (si *SqliteIndexer) applyMissingTipset(ctx context.Context, tx *sql.Tx, currTs *types.TipSet) error {
	if currTs == nil {
		return xerrors.Errorf("failed to apply missing tipset: tipset is nil")
	}

	// Special handling for genesis tipset
	if currTs.Height() == 0 {
		if err := si.indexTipset(ctx, tx, currTs); err != nil {
			return xerrors.Errorf("failed to index genesis tipset: %w", err)
		}
		return nil
	}

	parentTs, err := si.cs.GetTipSetFromKey(ctx, currTs.Parents())
	if err != nil {
		return xerrors.Errorf("failed to get parent tipset: %w", err)
	}

	// Index the tipset along with its parent events
	if err := si.indexTipsetWithParentEvents(ctx, tx, parentTs, currTs); err != nil {
		return xerrors.Errorf("failed to index tipset with parent events: %w", err)
	}

	return nil
}
