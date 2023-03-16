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
	// the tipset where this message was included
	TipSet cid.Cid
	// the epoch where this message was included
	Epoch abi.ChainEpoch
}

// MsgIndex is the interface to the message index
type MsgIndex interface {
	// GetMsgInfo retrieves the message metadata through the index.
	// The lookup is done using the onchain message Cid; that is the signed message Cid
	// for SECP messages and unsigned message Cid for BLS messages.
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
