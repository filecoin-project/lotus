package chainindex

import (
	"context"
	"database/sql"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

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
	si.closeLk.RLock()
	if si.closed {
		si.closeLk.RUnlock()
		return ErrClosed
	}
	si.closeLk.RUnlock()

	if head == nil {
		return nil
	}

	return withTx(ctx, si.db, func(tx *sql.Tx) error {
		var hasTipset bool
		err := tx.StmtContext(ctx, si.isTipsetMessageNonEmptyStmt).QueryRowContext(ctx).Scan(&hasTipset)
		if err != nil {
			return xerrors.Errorf("failed to check if tipset message is empty: %w", err)
		}

		isIndexEmpty := !hasTipset
		if isIndexEmpty && !si.reconcileEmptyIndex {
			log.Info("Chain index is empty and reconcileEmptyIndex is disabled; skipping reconciliation")
			return nil
		}

		if isIndexEmpty {
			log.Info("Chain index is empty; backfilling from head")
			return si.backfillEmptyIndex(ctx, tx, head)
		}

		// Find the minimum applied tipset in the index; this will mark the absolute min height of the reconciliation walk
		var reconciliationEpoch abi.ChainEpoch
		row := tx.StmtContext(ctx, si.getMinNonRevertedHeightStmt).QueryRowContext(ctx)
		if err := row.Scan(&reconciliationEpoch); err != nil {
			return xerrors.Errorf("failed to scan minimum non-reverted height: %w", err)
		}

		currTs := head
		log.Infof("Starting chain reconciliation from head height %d; searching for base reconciliation height above %d)", head.Height(), reconciliationEpoch)
		var missingTipsets []*types.TipSet

		// The goal here is to walk the canonical chain backwards from head until we find a matching non-reverted tipset
		// in the db so we know where to start reconciliation from
		// All tipsets that exist in the DB but not in the canonical chain are then marked as reverted
		// All tpsets that exist in the canonical chain but not in the db are then applied

		// we only need to walk back as far as the reconciliation epoch as all the tipsets in the index
		// below the reconciliation epoch are already marked as reverted because the reconciliation epoch
		// is the minimum non-reverted height in the index
		for currTs != nil && currTs.Height() >= reconciliationEpoch {
			tsKeyCidBytes, err := toTipsetKeyCidBytes(currTs)
			if err != nil {
				return xerrors.Errorf("failed to compute tipset cid: %w", err)
			}

			var exists bool
			err = tx.StmtContext(ctx, si.hasNonRevertedTipsetStmt).QueryRowContext(ctx, tsKeyCidBytes).Scan(&exists)
			if err != nil {
				return xerrors.Errorf("failed to check if tipset exists and is not reverted: %w", err)
			}

			if exists {
				// found it!
				reconciliationEpoch = currTs.Height() + 1
				log.Infof("Found matching tipset at height %d, setting reconciliation epoch to %d", currTs.Height(), reconciliationEpoch)
				break
			}

			if len(missingTipsets) < si.maxReconcileTipsets {
				missingTipsets = append(missingTipsets, currTs)
			}
			// even if len(missingTipsets) >= si.maxReconcileTipsets, we still need to continue the walk
			// to find the final reconciliation epoch so we can mark the indexed tipsets not in the main chain as reverted

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
		result, err := tx.StmtContext(ctx, si.updateTipsetsToRevertedFromHeightStmt).ExecContext(ctx, int64(reconciliationEpoch))
		if err != nil {
			return xerrors.Errorf("failed to mark tipsets as reverted: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return xerrors.Errorf("failed to get number of rows affected: %w", err)
		}

		// also need to mark events as reverted for the corresponding inclusion tipsets
		if _, err = tx.StmtContext(ctx, si.updateEventsToRevertedFromHeightStmt).ExecContext(ctx, int64(reconciliationEpoch-1)); err != nil {
			return xerrors.Errorf("failed to mark events as reverted: %w", err)
		}

		log.Infof("Marked %d tipsets as reverted from height %d", rowsAffected, reconciliationEpoch)

		return si.applyMissingTipsets(ctx, tx, missingTipsets)
	})
}

func (si *SqliteIndexer) backfillEmptyIndex(ctx context.Context, tx *sql.Tx, head *types.TipSet) error {
	currTs := head
	var missingTipsets []*types.TipSet

	log.Infof("Backfilling empty chain index from head height %d", head.Height())
	var err error

	for currTs != nil && len(missingTipsets) < si.maxReconcileTipsets {
		missingTipsets = append(missingTipsets, currTs)
		if currTs.Height() == 0 {
			break
		}

		currTs, err = si.cs.GetTipSetFromKey(ctx, currTs.Parents())
		if err != nil {
			return xerrors.Errorf("failed to walk chain: %w", err)
		}
	}

	return si.applyMissingTipsets(ctx, tx, missingTipsets)
}

func (si *SqliteIndexer) applyMissingTipsets(ctx context.Context, tx *sql.Tx, missingTipsets []*types.TipSet) error {
	if len(missingTipsets) == 0 {
		log.Info("No missing tipsets to index; index is all caught up with the chain")
		return nil
	}

	log.Infof("Applying %d missing tipsets to Index; max missing tipset height %d; min missing tipset height %d", len(missingTipsets),
		missingTipsets[0].Height(), missingTipsets[len(missingTipsets)-1].Height())
	totalIndexed := 0

	// apply all missing tipsets from the canonical chain to the current chain head
	for i := 0; i < len(missingTipsets); i++ {
		currTs := missingTipsets[i]
		var parentTs *types.TipSet
		var err error

		if i < len(missingTipsets)-1 {
			// a caller must supply a reverse-ordered contiguous list of missingTipsets
			parentTs = missingTipsets[i+1]
		} else if currTs.Height() > 0 {
			parentTs, err = si.cs.GetTipSetFromKey(ctx, currTs.Parents())
			if err != nil {
				return xerrors.Errorf("failed to get parent tipset: %w", err)
			}
		} else if currTs.Height() == 0 {
			if err := si.indexTipset(ctx, tx, currTs); err != nil {
				log.Warnf("failed to index genesis tipset during reconciliation: %s", err)
			} else {
				totalIndexed++
			}
			break
		}

		if err := si.indexTipsetWithParentEvents(ctx, tx, parentTs, currTs); err != nil {
			log.Warnf("failed to index tipset with parent events during reconciliation: %s", err)
			// the above could have failed because of missing messages for `parentTs` in the chainstore
			// so try to index only the currentTs and then halt the reconciliation process as we've
			// reached the end of what we have in the chainstore
			if err := si.indexTipset(ctx, tx, currTs); err != nil {
				log.Warnf("failed to index tipset during reconciliation: %s", err)
			} else {
				totalIndexed++
			}
			break
		}

		totalIndexed++
	}

	log.Infof("Indexed %d missing tipsets during reconciliation", totalIndexed)

	return nil
}
