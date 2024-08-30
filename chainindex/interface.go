package chainindex

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var ErrNotFound = errors.New("message not found")
var ErrClosed = errors.New("index closed")

// MsgInfo is the Message metadata the index tracks.
type MsgInfo struct {
	// the message this record refers to
	Message cid.Cid
	// the tipset where this message was included
	TipSet cid.Cid
	// the epoch where this message was included
	Epoch abi.ChainEpoch
}

type Indexer interface {
	ReconcileWithChain(ctx context.Context, currHead *types.TipSet) error
	IndexSignedMessage(ctx context.Context, msg *types.SignedMessage) error
	IndexEthTxHash(ctx context.Context, txHash ethtypes.EthHash, c cid.Cid) error

	Apply(ctx context.Context, from, to *types.TipSet) error
	Revert(ctx context.Context, from, to *types.TipSet) error

	// Returns (cid.Undef, nil) if the message was not found
	GetCidFromHash(ctx context.Context, hash ethtypes.EthHash) (cid.Cid, error)
	// Returns (nil, ErrNotFound) if the message was not found
	GetMsgInfo(ctx context.Context, m cid.Cid) (*MsgInfo, error)
	Close() error
}

type ChainStore interface {
	MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error)
	GetHeaviestTipSet() *types.TipSet
	GetTipSetByCid(ctx context.Context, tsKeyCid cid.Cid) (*types.TipSet, error)
	GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	MessagesForBlock(ctx context.Context, b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error)
}

var _ ChainStore = (*store.ChainStore)(nil)

type dummyIndexer struct{}

func (dummyIndexer) Close() error {
	return nil
}

func (dummyIndexer) IndexSignedMessage(ctx context.Context, msg *types.SignedMessage) error {
	return nil
}

func (dummyIndexer) IndexEthTxHash(ctx context.Context, txHash ethtypes.EthHash, c cid.Cid) error {
	return nil
}

func (dummyIndexer) GetCidFromHash(ctx context.Context, hash ethtypes.EthHash) (cid.Cid, error) {
	return cid.Undef, ErrNotFound
}

func (dummyIndexer) GetMsgInfo(ctx context.Context, m cid.Cid) (*MsgInfo, error) {
	return nil, ErrNotFound
}

func (dummyIndexer) Apply(ctx context.Context, from, to *types.TipSet) error {
	return nil
}

func (dummyIndexer) Revert(ctx context.Context, from, to *types.TipSet) error {
	return nil
}

func (dummyIndexer) ReconcileWithChain(ctx context.Context, currHead *types.TipSet) error {
	return nil
}

var DummyIndexer Indexer = dummyIndexer{}
