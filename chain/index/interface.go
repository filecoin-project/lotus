package index

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
)

// MsgInfo is the Message metadata the index tracks.
type MsgInfo struct {
	// the message this record refers to
	Message cid.Cid
	// the epoch whre this message was executed
	Epoch abi.ChainEpoch
	// the tipset where this messages executed
	Tipset cid.Cid
	// the first block in the tipset where the message was executed
	Block cid.Cid
	// the index of the message in the block
	Index int
}

// MsgIndex is the interface to the message index
type MsgIndex interface {
	// GetMsgInfo retrieves the message metadata through the index.
	GetMsgInfo(ctx context.Context, m cid.Cid) (MsgInfo, error)
	// Close closes the index
	Close() error
}
