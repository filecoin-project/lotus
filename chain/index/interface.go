package index

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
)

var ErrNotFound = errors.New("message not found")
var ErrClosed = errors.New("index closed")

// MsgInfo is the Message metadata the index tracks.
type MsgInfo struct {
	// the message this record refers to
	Message cid.Cid
	// the tipset where this messages was executed
	TipSet cid.Cid
	// the epoch whre this message was executed
	Epoch abi.ChainEpoch
	// the canonical execution order of the message in the tipset
	Index int
}

// MsgIndex is the interface to the message index
type MsgIndex interface {
	// GetMsgInfo retrieves the message metadata through the index.
	GetMsgInfo(ctx context.Context, m cid.Cid) (MsgInfo, error)
	// Close closes the index
	Close() error
}

type dummyMsgIndex struct{}

func (dummyMsgIndex) GetMsgInfo(ctx context.Context, m cid.Cid) (MsgInfo, error) {
	return MsgInfo{}, ErrNotFound
}

func (dummyMsgIndex) Close() error {
	return nil
}

var DummyMsgIndex MsgIndex = dummyMsgIndex{}
