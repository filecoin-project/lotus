package index

import (
	"context"
	"errors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
)

var ErrNotFound = errors.New("message not found")

// MsgInfo is the Message metadata the index tracks.
type MsgInfo struct {
	// the message this record refers to
	Message cid.Cid
	// the tipset where this messages was executed
	Tipset cid.Cid
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
