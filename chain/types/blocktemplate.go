package types

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

type BlockTemplate struct {
	Miner            address.Address
	Parents          TipSetKey
	Ticket           *Ticket
	Eproof           *ElectionProof
	BeaconValues     []BeaconEntry
	Messages         []*SignedMessage
	Epoch            abi.ChainEpoch
	Timestamp        uint64
	WinningPoStProof []builtin.PoStProof
}
