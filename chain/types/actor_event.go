package types

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
)

type ActorEventBlock struct {
	// what value codec does client want to match on ?
	Codec uint64
	// data associated with the "event key"
	Value []byte
}

type ActorEventFilter struct {
	// Matches events from one of these actors, or any actor if empty.
	Addresses []address.Address
	// Matches events with the specified key/values, or all events if empty.
	// If the `Blocks` slice is empty, matches on the key only.
	Fields map[string][]ActorEventBlock
	// Epoch based filtering ?
	FromBlock abi.ChainEpoch `json:"fromBlock,omitempty"`
	ToBlock   abi.ChainEpoch `json:"toBlock,omitempty"`
}

type ActorEvent struct {
	Entries     []EventEntry
	EmitterAddr address.Address // f4 address of emitter
	Reverted    bool
	Height      abi.ChainEpoch
	TipSetKey   cid.Cid // tipset that contained the message
	MsgCid      cid.Cid // cid of message that produced event
}
