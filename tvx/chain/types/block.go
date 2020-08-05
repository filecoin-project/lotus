package types

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
)

// BlockMessagesInfo contains messages for one block in a tipset.
type BlockMessagesInfo struct {
	BLSMessages  []*types.Message
	SECPMessages []*types.SignedMessage
	Miner        address.Address
	TicketCount  int64
}
