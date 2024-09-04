package chainindex

import (
	"context"
	"database/sql"
	"time"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var (
	headIndexedWaitTimeout = 5 * time.Second
)

func (si *SqliteIndexer) GetCidFromHash(ctx context.Context, txHash ethtypes.EthHash) (cid.Cid, error) {
	si.closeLk.RLock()
	if si.closed {
		return cid.Undef, ErrClosed
	}
	si.closeLk.RUnlock()

	var msgCidBytes []byte

	err := si.queryMsgCidFromEthHash(ctx, txHash, &msgCidBytes)
	if err == sql.ErrNoRows {
		err = si.waitTillHeadIndexedAndApply(ctx, func() error {
			return si.queryMsgCidFromEthHash(ctx, txHash, &msgCidBytes)
		})
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return cid.Undef, ErrNotFound
		}
		return cid.Undef, xerrors.Errorf("failed to get message CID from eth hash: %w", err)
	}

	msgCid, err := cid.Cast(msgCidBytes)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to cast message CID: %w", err)
	}

	return msgCid, nil
}

func (si *SqliteIndexer) queryMsgCidFromEthHash(ctx context.Context, txHash ethtypes.EthHash, msgCidBytes *[]byte) error {
	return si.getMsgCidFromEthHashStmt.QueryRowContext(ctx, txHash.String()).Scan(msgCidBytes)
}

func (si *SqliteIndexer) GetMsgInfo(ctx context.Context, messageCid cid.Cid) (*MsgInfo, error) {
	si.closeLk.RLock()
	if si.closed {
		return nil, ErrClosed
	}
	si.closeLk.RUnlock()

	var tipsetKeyCidBytes []byte
	var height int64

	err := si.queryMsgInfo(ctx, messageCid, &tipsetKeyCidBytes, &height)
	if err == sql.ErrNoRows {
		err = si.waitTillHeadIndexedAndApply(ctx, func() error {
			return si.queryMsgInfo(ctx, messageCid, &tipsetKeyCidBytes, &height)
		})
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, xerrors.Errorf("failed to get message info: %w", err)
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

func (si *SqliteIndexer) queryMsgInfo(ctx context.Context, messageCid cid.Cid, tipsetKeyCidBytes *[]byte, height *int64) error {
	return si.getNonRevertedMsgInfoStmt.QueryRowContext(ctx, messageCid.Bytes()).Scan(tipsetKeyCidBytes, height)
}

func (si *SqliteIndexer) isTipsetIndexed(ctx context.Context, tsKeyCid []byte) (bool, error) {
	var exists bool
	err := si.tipsetExistsStmt.QueryRowContext(ctx, tsKeyCid).Scan(&exists)
	if err != nil {
		return false, xerrors.Errorf("error checking if tipset exists: %w", err)
	}
	return exists, nil
}

func (si *SqliteIndexer) waitTillHeadIndexedAndApply(ctx context.Context, applyFn func() error) error {
	ctx, cancel := context.WithTimeout(ctx, headIndexedWaitTimeout)
	defer cancel()

	head := si.cs.GetHeaviestTipSet()
	headTsKeyCidBytes, err := toTipsetKeyCidBytes(head)
	if err != nil {
		return xerrors.Errorf("error getting tipset key cid: %w", err)
	}

	// is it already indexed?
	if exists, err := si.isTipsetIndexed(ctx, headTsKeyCidBytes); err != nil {
		return xerrors.Errorf("error checking if tipset exists: %w", err)
	} else if exists {
		return applyFn()
	}

	// wait till it is indexed
	subCh, unsubFn := si.subscribeUpdates()
	defer unsubFn()

	for {
		exists, err := si.isTipsetIndexed(ctx, headTsKeyCidBytes)
		if err != nil {
			return xerrors.Errorf("error checking if tipset exists: %w", err)
		}
		if exists {
			return applyFn()
		}

		select {
		case <-subCh:
			// Continue to next iteration to check again
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
