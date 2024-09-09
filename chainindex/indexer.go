package chainindex

import (
	"context"
	"database/sql"
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/lib/sqlite"
)

var _ Indexer = (*SqliteIndexer)(nil)

// IdToRobustAddrFunc is a function type that resolves an actor ID to a robust address
type IdToRobustAddrFunc func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool)

type SqliteIndexer struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	db *sql.DB
	cs ChainStore

	idToRobustAddrFunc IdToRobustAddrFunc

	insertEthTxHashStmt                   *sql.Stmt
	getNonRevertedMsgInfoStmt             *sql.Stmt
	getMsgCidFromEthHashStmt              *sql.Stmt
	insertTipsetMessageStmt               *sql.Stmt
	updateTipsetToRevertedStmt            *sql.Stmt
	getMaxNonRevertedTipsetStmt           *sql.Stmt
	hasTipsetStmt                         *sql.Stmt
	updateTipsetToNonRevertedStmt         *sql.Stmt
	removeRevertedTipsetsBeforeHeightStmt *sql.Stmt
	removeTipsetsBeforeHeightStmt         *sql.Stmt
	removeEthHashesOlderThanStmt          *sql.Stmt
	updateTipsetsToRevertedFromHeightStmt *sql.Stmt
	updateEventsToRevertedFromHeightStmt  *sql.Stmt
	isTipsetMessageNonEmptyStmt           *sql.Stmt
	getMinNonRevertedHeightStmt           *sql.Stmt
	hasNonRevertedTipsetStmt              *sql.Stmt
	updateEventsToRevertedStmt            *sql.Stmt
	updateEventsToNonRevertedStmt         *sql.Stmt
	getMsgIdForMsgCidAndTipsetStmt        *sql.Stmt
	insertEventStmt                       *sql.Stmt
	insertEventEntryStmt                  *sql.Stmt

	gcRetentionDays     int64
	reconcileEmptyIndex bool
	maxReconcileTipsets int

	mu           sync.Mutex
	updateSubs   map[uint64]*updateSub
	subIdCounter uint64

	closeLk sync.RWMutex
	closed  bool
}

func NewSqliteIndexer(path string, cs ChainStore, gcRetentionDays int64, reconcileEmptyIndex bool,
	maxReconcileTipsets int) (si *SqliteIndexer, err error) {
	db, _, err := sqlite.Open(path)
	if err != nil {
		return nil, xerrors.Errorf("failed to setup message index db: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	defer func() {
		if err != nil {
			_ = db.Close()
			cancel()
		}
	}()

	err = sqlite.InitDb(ctx, "chain index", db, ddls, []sqlite.MigrationFunc{})
	if err != nil {
		return nil, xerrors.Errorf("failed to init message index db: %w", err)
	}

	si = &SqliteIndexer{
		ctx:                 ctx,
		cancel:              cancel,
		db:                  db,
		cs:                  cs,
		updateSubs:          make(map[uint64]*updateSub),
		subIdCounter:        0,
		gcRetentionDays:     gcRetentionDays,
		reconcileEmptyIndex: reconcileEmptyIndex,
		maxReconcileTipsets: maxReconcileTipsets,
	}
	if err = si.prepareStatements(); err != nil {
		return nil, xerrors.Errorf("failed to prepare statements: %w", err)
	}

	si.wg.Add(1)
	go si.gcLoop()

	return si, nil
}

func (si *SqliteIndexer) SetIdToRobustAddrFunc(idToRobustAddrFunc IdToRobustAddrFunc) {
	si.idToRobustAddrFunc = idToRobustAddrFunc
}

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

		// Find the minimum applied tipset in the index; this will mark the absolute min height of the reconciliation walk
		var reconciliationEpoch abi.ChainEpoch
		if isIndexEmpty {
			reconciliationEpoch = 0
		} else {
			var result int64
			row := tx.StmtContext(ctx, si.getMinNonRevertedHeightStmt).QueryRowContext(ctx)
			if err := row.Scan(&result); err != nil {
				return xerrors.Errorf("failed to scan minimum non-reverted height %w", err)
			}
			reconciliationEpoch = abi.ChainEpoch(result)
		}

		currTs := head
		log.Infof("Starting chain reconciliation from height %d, reconciliationEpoch: %d", head.Height(), reconciliationEpoch)
		var missingTipsets []*types.TipSet

		// The goal here is to walk the canonical chain backwards from head until we find a matching non-reverted tipset
		// in the db so we know where to start reconciliation from
		// All tipsets that exist in the DB but not in the canonical chain are then marked as reverted
		// All tpsets that exist in the canonical chain but not in the db are then applied
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

			if len(missingTipsets) <= si.maxReconcileTipsets {
				missingTipsets = append(missingTipsets, currTs)
			}

			if currTs.Height() == 0 {
				break
			}

			parents := currTs.Parents()
			currTs, err = si.cs.GetTipSetFromKey(ctx, parents)
			if err != nil {
				return xerrors.Errorf("failed to walk chain: %w", err)
			}
		}

		if currTs == nil {
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
		log.Infof("Marked %d tipsets as reverted from height %d", rowsAffected, reconciliationEpoch)

		// also need to mark events as reverted for the corresponding inclusion tipsets
		if _, err = tx.StmtContext(ctx, si.updateEventsToRevertedFromHeightStmt).ExecContext(ctx, int64(reconciliationEpoch-1)); err != nil {
			return xerrors.Errorf("failed to mark events as reverted: %w", err)
		}

		totalIndexed := 0
		// apply all missing tipsets from the canonical chain to the current chain head
		for i := 0; i < len(missingTipsets); i++ {
			currTs := missingTipsets[i]
			var parentTs *types.TipSet
			var err error

			if i < len(missingTipsets)-1 {
				parentTs = missingTipsets[i+1]
			} else if currTs.Height() > 0 {
				parentTs, err = si.cs.GetTipSetFromKey(ctx, currTs.Parents())
				if err != nil {
					return xerrors.Errorf("failed to get parent tipset: %w", err)
				}
			} else if currTs.Height() == 0 {
				if err := si.indexTipset(ctx, tx, currTs); err != nil {
					log.Warnf("failed to index tipset during reconciliation: %s", err)
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
				}
				break
			}
			totalIndexed++
		}

		log.Infof("Indexed %d missing tipsets during reconciliation", totalIndexed)

		return nil
	})
}

func (si *SqliteIndexer) Close() error {
	si.closeLk.Lock()
	defer si.closeLk.Unlock()
	if si.closed {
		return nil
	}
	si.closed = true

	if si.db == nil {
		return nil
	}
	si.cancel()
	si.wg.Wait()

	if err := si.db.Close(); err != nil {
		return xerrors.Errorf("failed to close db: %w", err)
	}
	return nil
}

func (si *SqliteIndexer) prepareStatements() error {
	var err error

	si.insertEthTxHashStmt, err = si.db.Prepare(stmtInsertEthTxHash)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "insertEthTxHashStmt", err)
	}

	si.getNonRevertedMsgInfoStmt, err = si.db.Prepare(stmtGetNonRevertedMessageInfo)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "getNonRevertedMsgInfoStmt", err)
	}

	si.getMsgCidFromEthHashStmt, err = si.db.Prepare(stmtGetMsgCidFromEthHash)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "getMsgCidFromEthHashStmt", err)
	}

	si.insertTipsetMessageStmt, err = si.db.Prepare(stmtInsertTipsetMessage)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "insertTipsetMessageStmt", err)
	}

	si.hasTipsetStmt, err = si.db.Prepare(stmtHasTipset)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "hasTipsetStmt", err)
	}

	si.updateTipsetToNonRevertedStmt, err = si.db.Prepare(stmtUpdateTipsetToNonReverted)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "updateTipsetToNonRevertedStmt", err)
	}

	si.updateTipsetToRevertedStmt, err = si.db.Prepare(stmtUpdateTipsetToReverted)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "updateTipsetToRevertedStmt", err)
	}

	si.getMaxNonRevertedTipsetStmt, err = si.db.Prepare(stmtGetMaxNonRevertedTipset)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "getMaxNonRevertedTipsetStmt", err)
	}

	si.removeRevertedTipsetsBeforeHeightStmt, err = si.db.Prepare(stmtRemoveRevertedTipsetsBeforeHeight)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "removeRevertedTipsetsBeforeHeightStmt", err)
	}

	si.removeTipsetsBeforeHeightStmt, err = si.db.Prepare(stmtRemoveTipsetsBeforeHeight)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "removeTipsetsBeforeHeightStmt", err)
	}

	si.removeEthHashesOlderThanStmt, err = si.db.Prepare(stmtRemoveEthHashesOlderThan)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "removeEthHashesOlderThanStmt", err)
	}

	si.updateTipsetsToRevertedFromHeightStmt, err = si.db.Prepare(stmtUpdateTipsetsToRevertedFromHeight)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "updateTipsetsToRevertedFromHeightStmt", err)
	}

	si.updateEventsToRevertedFromHeightStmt, err = si.db.Prepare(stmtUpdateEventsToRevertedFromHeight)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "updateEventsToRevertedFromHeightStmt", err)
	}

	si.isTipsetMessageNonEmptyStmt, err = si.db.Prepare(stmtIsTipsetMessageNonEmpty)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "isTipsetMessageNonEmptyStmt", err)
	}

	si.getMinNonRevertedHeightStmt, err = si.db.Prepare(stmtGetMinNonRevertedHeight)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "getMinNonRevertedHeightStmt", err)
	}

	si.hasNonRevertedTipsetStmt, err = si.db.Prepare(stmtHasNonRevertedTipset)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "hasNonRevertedTipsetStmt", err)
	}

	si.updateEventsToNonRevertedStmt, err = si.db.Prepare(stmtUpdateEventsToNonReverted)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "updateEventsToNonRevertedStmt", err)
	}

	si.updateEventsToRevertedStmt, err = si.db.Prepare(stmtUpdateEventsToReverted)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "updateEventsToRevertedStmt", err)
	}

	si.getMsgIdForMsgCidAndTipsetStmt, err = si.db.Prepare(stmtGetMsgIdForMsgCidAndTipset)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "getMsgIdForMsgCidAndTipsetStmt", err)
	}

	si.insertEventStmt, err = si.db.Prepare(stmtInsertEvent)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "insertEventStmt", err)
	}

	si.insertEventEntryStmt, err = si.db.Prepare(stmtInsertEventEntry)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "insertEventEntryStmt", err)
	}

	return nil
}

func (si *SqliteIndexer) IndexEthTxHash(ctx context.Context, txHash ethtypes.EthHash, msgCid cid.Cid) error {
	si.closeLk.RLock()
	if si.closed {
		return ErrClosed
	}
	si.closeLk.RUnlock()

	return withTx(ctx, si.db, func(tx *sql.Tx) error {
		return si.indexEthTxHash(ctx, tx, txHash, msgCid)
	})
}

func (si *SqliteIndexer) IndexSignedMessage(ctx context.Context, msg *types.SignedMessage) error {
	if msg.Signature.Type != crypto.SigTypeDelegated {
		return nil
	}
	si.closeLk.RLock()
	if si.closed {
		return ErrClosed
	}
	si.closeLk.RUnlock()

	return withTx(ctx, si.db, func(tx *sql.Tx) error {
		return si.indexSignedMessage(ctx, tx, msg)
	})
}

func (si *SqliteIndexer) indexSignedMessage(ctx context.Context, tx *sql.Tx, msg *types.SignedMessage) error {
	ethTx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(msg)
	if err != nil {
		return xerrors.Errorf("error converting filecoin message to eth tx: %w", err)
	}

	txHash, err := ethTx.TxHash()
	if err != nil {
		return xerrors.Errorf("error hashing transaction: %w", err)
	}

	return si.indexEthTxHash(ctx, tx, txHash, msg.Cid())
}

func (si *SqliteIndexer) indexEthTxHash(ctx context.Context, tx *sql.Tx, txHash ethtypes.EthHash, msgCid cid.Cid) error {
	insertEthTxHashStmt := tx.Stmt(si.insertEthTxHashStmt)
	_, err := insertEthTxHashStmt.ExecContext(ctx, txHash.String(), msgCid.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to index eth tx hash: %w", err)
	}

	return nil
}

func (si *SqliteIndexer) Apply(ctx context.Context, from, to *types.TipSet) error {
	si.closeLk.RLock()
	if si.closed {
		return ErrClosed
	}
	si.closeLk.RUnlock()

	// We're moving the chain ahead from the `from` tipset to the `to` tipset
	// Height(to) > Height(from)
	err := withTx(ctx, si.db, func(tx *sql.Tx) error {
		if err := si.indexTipsetWithParentEvents(ctx, tx, from, to); err != nil {
			return xerrors.Errorf("error indexing tipset: %w", err)
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("error applying tipset: %w", err)
	}

	si.notifyUpdateSubs()

	return nil
}

func (si *SqliteIndexer) Revert(ctx context.Context, from, to *types.TipSet) error {
	si.closeLk.RLock()
	if si.closed {
		return ErrClosed
	}
	si.closeLk.RUnlock()

	// We're reverting the chain from the tipset at `from` to the tipset at `to`.
	// Height(to) < Height(from)

	revertTsKeyCid, err := toTipsetKeyCidBytes(from)
	if err != nil {
		return xerrors.Errorf("error getting tipset key cid: %w", err)
	}

	// Because of deferred execution in Filecoin, events at tipset T are reverted when a tipset T+1 is reverted.
	// However, the tipet `T` itself is not reverted.
	eventTsKeyCid, err := toTipsetKeyCidBytes(to)
	if err != nil {
		return xerrors.Errorf("error getting tipset key cid: %w", err)
	}

	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		if _, err := tx.Stmt(si.updateTipsetToRevertedStmt).ExecContext(ctx, revertTsKeyCid); err != nil {
			return xerrors.Errorf("error marking tipset %s as reverted: %w", revertTsKeyCid, err)
		}

		// events are indexed against the message inclusion tipset, not the message execution tipset.
		// So we need to revert the events for the message inclusion tipset.
		if _, err := tx.Stmt(si.updateEventsToRevertedStmt).ExecContext(ctx, eventTsKeyCid); err != nil {
			return xerrors.Errorf("error reverting events for tipset %s: %w", eventTsKeyCid, err)
		}

		return nil
	})
	if err != nil {
		return xerrors.Errorf("error during revert transaction: %w", err)
	}

	si.notifyUpdateSubs()

	return nil
}

func (si *SqliteIndexer) indexTipset(ctx context.Context, tx *sql.Tx, ts *types.TipSet) error {
	tsKeyCidBytes, err := toTipsetKeyCidBytes(ts)
	if err != nil {
		return xerrors.Errorf("failed to compute tipset cid: %w", err)
	}

	if restored, err := si.restoreTipsetIfExists(ctx, tx, tsKeyCidBytes); err != nil {
		return xerrors.Errorf("failed to restore tipset: %w", err)
	} else if restored {
		return nil
	}

	height := ts.Height()
	insertTipsetMsgStmt := tx.Stmt(si.insertTipsetMessageStmt)

	msgs, err := si.cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return xerrors.Errorf("failed to get messages for tipset: %w", err)
	}

	if len(msgs) == 0 {
		// If there are no messages, just insert the tipset and return
		if _, err := insertTipsetMsgStmt.ExecContext(ctx, tsKeyCidBytes, height, 0, nil, -1); err != nil {
			return xerrors.Errorf("failed to insert empty tipset: %w", err)
		}
		return nil
	}

	for i, msg := range msgs {
		msg := msg
		if _, err := insertTipsetMsgStmt.ExecContext(ctx, tsKeyCidBytes, height, 0, msg.Cid().Bytes(), i); err != nil {
			return xerrors.Errorf("failed to insert tipset message: %w", err)
		}
	}

	for _, blk := range ts.Blocks() {
		blk := blk
		_, smsgs, err := si.cs.MessagesForBlock(ctx, blk)
		if err != nil {
			return xerrors.Errorf("failed to get messages for block: %w", err)
		}

		for _, smsg := range smsgs {
			smsg := smsg
			if smsg.Signature.Type != crypto.SigTypeDelegated {
				continue
			}
			if err := si.indexSignedMessage(ctx, tx, smsg); err != nil {
				return xerrors.Errorf("failed to index eth tx hash: %w", err)
			}
		}
	}

	return nil
}

func (si *SqliteIndexer) indexTipsetWithParentEvents(ctx context.Context, tx *sql.Tx, parentTs *types.TipSet, currentTs *types.TipSet) error {
	// Index the parent tipset if it doesn't exist yet.
	// This is necessary to properly index events produced by executing
	// messages included in the parent tipset by the current tipset (deferred execution).
	if err := si.indexTipset(ctx, tx, parentTs); err != nil {
		return xerrors.Errorf("failed to index parent tipset: %w", err)
	}
	if err := si.indexTipset(ctx, tx, currentTs); err != nil {
		return xerrors.Errorf("failed to index tipset: %w", err)
	}

	// Now Index events
	if err := si.indexEvents(ctx, tx, parentTs, currentTs); err != nil {
		return xerrors.Errorf("failed to index events: %w", err)
	}

	return nil
}

func (si *SqliteIndexer) restoreTipsetIfExists(ctx context.Context, tx *sql.Tx, tsKeyCidBytes []byte) (bool, error) {
	// Check if the tipset already exists
	var exists bool
	if err := tx.Stmt(si.hasTipsetStmt).QueryRowContext(ctx, tsKeyCidBytes).Scan(&exists); err != nil {
		return false, xerrors.Errorf("failed to check if tipset exists: %w", err)
	}
	if exists {
		if _, err := tx.Stmt(si.updateTipsetToNonRevertedStmt).ExecContext(ctx, tsKeyCidBytes); err != nil {
			return false, xerrors.Errorf("failed to restore tipset: %w", err)
		}
		return true, nil
	}
	return false, nil
}
