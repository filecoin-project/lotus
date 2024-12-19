package index

import (
	"context"
	"database/sql"
	"time"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

const headIndexedWaitTimeout = 5 * time.Second

func (si *SqliteIndexer) GetCidFromHash(ctx context.Context, txHash ethtypes.EthHash) (cid.Cid, error) {
	if si.isClosed() {
		return cid.Undef, ErrClosed
	}

	var msgCidBytes []byte

	if err := si.readWithHeadIndexWait(ctx, func() error {
		return si.queryMsgCidFromEthHash(ctx, txHash, &msgCidBytes)
	}); err != nil {
		return cid.Undef, err
	}

	msgCid, err := cid.Cast(msgCidBytes)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to cast message CID: %w", err)
	}

	return msgCid, nil
}

func (si *SqliteIndexer) queryMsgCidFromEthHash(ctx context.Context, txHash ethtypes.EthHash, msgCidBytes *[]byte) error {
	return si.stmts.getMsgCidFromEthHashStmt.QueryRowContext(ctx, txHash.String()).Scan(msgCidBytes)
}

func (si *SqliteIndexer) GetMsgInfo(ctx context.Context, messageCid cid.Cid) (*MsgInfo, error) {
	if si.isClosed() {
		return nil, ErrClosed
	}

	var tipsetKeyCidBytes []byte
	var height int64

	if err := si.queryMsgInfo(ctx, messageCid, &tipsetKeyCidBytes, &height); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	tipsetKey, err := cid.Cast(tipsetKeyCidBytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to cast tipset key cid: %w", err)
	}

	return &MsgInfo{
		Message: messageCid,
		TipSet:  tipsetKey,
		Epoch:   abi.ChainEpoch(height),
	}, nil
}

// This function attempts to read data using the provided readFunc.
// If the initial read returns no rows, it waits for the head to be indexed
// and tries again. This ensures that the most up-to-date data is checked.
// If no data is found after the second attempt, it returns ErrNotFound.
func (si *SqliteIndexer) readWithHeadIndexWait(ctx context.Context, readFunc func() error) error {
	err := readFunc()
	if err == sql.ErrNoRows {
		// not found, but may be in latest head, so wait for it and check again
		if err := si.waitTillHeadIndexed(ctx); err != nil {
			return xerrors.Errorf("failed while waiting for head to be indexed: %w", err)
		}
		err = readFunc()
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return xerrors.Errorf("failed to read data from index: %w", err)
	}

	return nil
}

func (si *SqliteIndexer) queryMsgInfo(ctx context.Context, messageCid cid.Cid, tipsetKeyCidBytes *[]byte, height *int64) error {
	return si.stmts.getNonRevertedMsgInfoStmt.QueryRowContext(ctx, messageCid.Bytes()).Scan(tipsetKeyCidBytes, height)
}

func (si *SqliteIndexer) waitTillHeadIndexed(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, headIndexedWaitTimeout)
	defer cancel()

	head := si.cs.GetHeaviestTipSet()
	headTsKeyCidBytes, err := toTipsetKeyCidBytes(head)
	if err != nil {
		return xerrors.Errorf("failed to get tipset key cid: %w", err)
	}

	// wait till it is indexed
	subCh, unsubFn := si.subscribeUpdates()
	defer unsubFn()

	for ctx.Err() == nil {
		exists, err := si.isTipsetIndexed(ctx, headTsKeyCidBytes)
		if err != nil {
			return xerrors.Errorf("failed to check if tipset exists: %w", err)
		} else if exists {
			return nil
		}

		select {
		case <-subCh:
			// Continue to next iteration to check again
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return ctx.Err()
}

func (si *SqliteIndexer) isTipsetIndexed(ctx context.Context, tsKeyCidBytes []byte) (bool, error) {
	var exists bool
	if err := si.stmts.hasTipsetStmt.QueryRowContext(ctx, tsKeyCidBytes).Scan(&exists); err != nil {
		return false, xerrors.Errorf("failed to check if tipset is indexed: %w", err)
	}
	return exists, nil
}
