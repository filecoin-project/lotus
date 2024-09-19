package index

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
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

	// we need to take a write lock here so that back-filling does not race with real time chain indexing
	si.writerLk.Lock()
	defer si.writerLk.Unlock()

	// this API only works for epoch < head because of deferred execution in Filecoin
	head := si.cs.GetHeaviestTipSet()
	if epoch >= head.Height() {
		return nil, xerrors.Errorf("cannot validate index at epoch %d, can only validate at an epoch less than chain head epoch %d", epoch, head.Height())
	}

	var isIndexEmpty bool
	err := si.stmts.isIndexEmptyStmt.QueryRowContext(ctx).Scan(&isIndexEmpty)
	if err != nil {
		return nil, xerrors.Errorf("failed to check if index is empty: %w", err)
	}

	// fetch the tipset at the given epoch on the canonical chain
	expectedTs, err := si.cs.GetTipsetByHeight(ctx, epoch, nil, true)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset at height %d: %w", epoch, err)
	}

	// Canonical chain has a null round at the epoch -> return if index is empty otherwise validate that index also
	// has a null round at this epoch i.e. it does not have anything indexed at all for this epoch
	if expectedTs.Height() != epoch {
		if isIndexEmpty {
			return &types.IndexValidation{
				Height:      uint64(epoch),
				IsNullRound: true,
			}, nil
		}
		// validate the db has a hole here and error if not, we don't attempt to repair because something must be very wrong for this to fail
		return si.validateIsNullRound(ctx, epoch)
	}

	// if the index is empty -> short-circuit and simply backfill if applicable
	if isIndexEmpty {
		return si.backfillMissingTipset(ctx, expectedTs, backfill)
	}

	// see if the tipset at this epoch is already indexed or if we need to backfill
	revertedCount, nonRevertedCount, err := si.getTipsetCountsAtHeight(ctx, epoch)
	if err != nil {
		if err == sql.ErrNoRows {
			return si.backfillMissingTipset(ctx, expectedTs, backfill)
		}
		return nil, xerrors.Errorf("failed to get tipset counts at height %d: %w", epoch, err)
	}

	switch {
	case revertedCount == 0 && nonRevertedCount == 0:
		// no tipsets at this epoch in the index, backfill
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

	// indexedTsKeyCid and expectedTsKeyCid are the same, so we can use `expectedTs` to fetch the indexed data
	indexedData, err := si.getIndexedTipSetData(ctx, expectedTs)
	if err != nil {
		return nil, xerrors.Errorf("failed to get indexed data for tipset at height %d: %w", expectedTs.Height(), err)
	}

	if indexedData == nil {
		return nil, xerrors.Errorf("nil indexed data for tipset at height %d", expectedTs.Height())
	}

	if err = si.verifyIndexedData(ctx, expectedTs, indexedData); err != nil {
		return nil, xerrors.Errorf("failed to verify indexed data at height %d: %w", expectedTs.Height(), err)
	}

	return &types.IndexValidation{
		TipSetKey:            expectedTs.Key(),
		Height:               uint64(expectedTs.Height()),
		IndexedMessagesCount: uint64(indexedData.nonRevertedMessageCount),
		IndexedEventsCount:   uint64(indexedData.nonRevertedEventCount),
		IndexedTxHashCount:   uint64(indexedData.txHashCount),
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
		Height:      uint64(epoch),
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
	nonRevertedMessageCount int
	nonRevertedEventCount   int
	txHashCount             int
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

		if err = tx.Stmt(si.stmts.getTxHashCountStmt).QueryRowContext(ctx, tsKeyCidBytes).Scan(&data.txHashCount); err != nil {
			return xerrors.Errorf("failed to query tx hash count: %w", err)
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
		return xerrors.Errorf("failed to get tipset key cid at height %d: %w", ts.Height(), err)
	}

	executionTs, err := si.getNextTipset(ctx, ts)
	if err != nil {
		return xerrors.Errorf("failed to get next tipset for height %d: %w", ts.Height(), err)
	}

	chainStoreData, err := si.getChainStoreTipsetData(ctx, ts, executionTs)
	if err != nil {
		return xerrors.Errorf("failed to get chain store data for tipset at height %d: %w", ts.Height(), err)
	}

	if chainStoreData == nil {
		return xerrors.Errorf("invalid chain store data for tipset at height %d", ts.Height())
	}

	if chainStoreData.totalMsgCount != indexedData.nonRevertedMessageCount {
		return xerrors.Errorf("message count mismatch: chainstore has %d, index has %d", chainStoreData.totalMsgCount, indexedData.nonRevertedMessageCount)
	}

	if chainStoreData.totalEventsCount != indexedData.nonRevertedEventCount {
		return xerrors.Errorf("event count mismatch: chainstore has %d, index has %d", chainStoreData.totalEventsCount, indexedData.nonRevertedEventCount)
	}

	// if non-reverted events exist which means that tipset `ts` has been executed, there should be 0 reverted events in the DB
	var hasRevertedEventsInTipset bool
	err = si.stmts.hasRevertedEventsInTipsetStmt.QueryRowContext(ctx, tsKeyCid.Bytes()).Scan(&hasRevertedEventsInTipset)
	if err != nil {
		return xerrors.Errorf("failed to check if there are reverted events in tipset for height %d: %w", ts.Height(), err)
	}
	if hasRevertedEventsInTipset {
		return xerrors.Errorf("index corruption: reverted events found for an executed tipset %s at height %d", tsKeyCid, ts.Height())
	}

	if chainStoreData.totalTxHashCount != indexedData.txHashCount {
		return xerrors.Errorf("tx hash count mismatch: chainstore has %d, index has %d", chainStoreData.totalTxHashCount, indexedData.txHashCount)
	}

	return nil
}

type chainStoreTipsetData struct {
	totalMsgCount    int
	totalEventsCount int
	totalTxHashCount int
}

func (si *SqliteIndexer) getChainStoreTipsetData(ctx context.Context, ts *types.TipSet, executionTs *types.TipSet) (*chainStoreTipsetData, error) {
	executedMsgs, err := si.loadExecutedMessages(ctx, ts, executionTs)
	if err != nil {
		return nil, xerrors.Errorf("failed to load executed messages: %w", err)
	}

	msgCount := len(executedMsgs)
	eventsCount := 0
	txHashCount := 0

	for _, emsg := range executedMsgs {
		eventsCount += len(emsg.evs)
	}

	for _, header := range ts.Blocks() {
		_, smsgs, err := si.cs.MessagesForBlock(ctx, header)
		if err != nil {
			return nil, xerrors.Errorf("failed to get messages for block: %w", err)
		}

		for _, smsg := range smsgs {
			if smsg.Signature.Type != crypto.SigTypeDelegated {
				continue
			}

			tx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(smsg)
			if err != nil {
				return nil, xerrors.Errorf("failed to convert from signed message: %w at epoch: %d", err, ts.Height())
			}

			if _, err = tx.TxHash(); err != nil {
				return nil, xerrors.Errorf("failed to calculate hash for ethTx: %w at epoch: %d", err, ts.Height())
			}

			txHashCount++
		}
	}

	return &chainStoreTipsetData{
		totalMsgCount:    msgCount,
		totalEventsCount: eventsCount,
		totalTxHashCount: txHashCount,
	}, nil
}

func (si *SqliteIndexer) backfillMissingTipset(ctx context.Context, ts *types.TipSet, backfill bool) (*types.IndexValidation, error) {
	if !backfill {
		return nil, xerrors.Errorf("missing tipset at height %d in the chain index, set backfill flag to true to fix", ts.Height())
	}
	// backfill the tipset in the Index
	parentTs, err := si.cs.GetTipSetFromKey(ctx, ts.Parents())
	if err != nil {
		return nil, xerrors.Errorf("failed to get parent tipset at height %d: %w", ts.Height(), err)
	}

	executionTs, err := si.getNextTipset(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get next tipset at height %d: %w", ts.Height(), err)
	}

	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		if err := si.indexTipsetWithParentEvents(ctx, tx, ts, executionTs); err != nil {
			return xerrors.Errorf("error indexing (ts, executionTs): %w", err)
		}

		if err := si.indexTipsetWithParentEvents(ctx, tx, parentTs, ts); err != nil {
			return xerrors.Errorf("error indexing (parentTs, ts): %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, xerrors.Errorf("failed to backfill tipset a: %w", err)
	}

	indexedData, err := si.getIndexedTipSetData(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get indexed tipset data: %w", err)
	}

	return &types.IndexValidation{
		TipSetKey:            ts.Key(),
		Height:               uint64(ts.Height()),
		Backfilled:           true,
		IndexedMessagesCount: uint64(indexedData.nonRevertedMessageCount),
		IndexedEventsCount:   uint64(indexedData.nonRevertedEventCount),
		IndexedTxHashCount:   uint64(indexedData.txHashCount),
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
