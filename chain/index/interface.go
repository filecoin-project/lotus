package index

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chainindex"
)

var ErrNotFound = errors.New("message not found")
var ErrClosed = errors.New("index closed")

// MsgIndex is the interface to the message index
type MsgIndex interface {
	// GetMsgInfo retrieves the message metadata through the index.
	// The lookup is done using the onchain message Cid; that is the signed message Cid
	// for SECP messages and unsigned message Cid for BLS messages.
	GetMsgInfo(ctx context.Context, m cid.Cid) (*chainindex.MsgInfo, error)
	// Close closes the index
	Close() error
}

type dummyMsgIndex struct{}

func (dummyMsgIndex) GetMsgInfo(ctx context.Context, m cid.Cid) (*chainindex.MsgInfo, error) {
	return nil, ErrNotFound
}

func (dummyMsgIndex) Close() error {
	return nil
}

var DummyMsgIndex MsgIndex = dummyMsgIndex{}
