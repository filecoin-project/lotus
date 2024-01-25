package types

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
)

type ActorEventBlock struct {
	// what value codec does client want to match on ?
	Codec uint64 `json:"codec"`
	// data associated with the "event key"
	Value []byte `json:"value"`
}

type SubActorEventFilter struct {
	Filter  ActorEventFilter `json:"filter"`
	Prefill bool             `json:"prefill"`
}

type ActorEventFilter struct {
	// Matches events from one of these actors, or any actor if empty.
	// For now, this MUST be a Filecoin address.
	Addresses []address.Address `json:"address"`

	// Matches events with the specified key/values, or all events if empty.
	// If the `Blocks` slice is empty, matches on the key only.
	Fields map[string][]ActorEventBlock `json:"fields"`

	// Epoch based filtering ?
	// Start epoch for the filter; -1 means no minimum
	MinEpoch abi.ChainEpoch `json:"minEpoch,omitempty"`

	// End epoch for the filter; -1 means no maximum
	MaxEpoch abi.ChainEpoch `json:"maxEpoch,omitempty"`
}

type ActorEvent struct {
	Entries     []EventEntry
	EmitterAddr address.Address
	Reverted    bool
	Height      abi.ChainEpoch
	TipSetKey   cid.Cid // tipset that contained the message
	MsgCid      cid.Cid // cid of message that produced event
}
