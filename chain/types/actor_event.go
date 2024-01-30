package types

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
)

type ActorEventBlock struct {
	// what value codec does client want to match on ?
	Codec uint64 `json:"codec"`

	// value we want to match on associated with the corresponding "event key"
	// should be a byte array encoded with the above codec
	// assumes base64 encoding for marshalling/um-marshalling byte arrays to json
	Value []byte `json:"value"`
}

type SubActorEventFilter struct {
	Filter ActorEventFilter `json:"filter"`

	// If true, historical events that match the given filter will be written to the response stream
	// before any new real-time events that match the given filter are written.
	Prefill bool `json:"prefill"`
}

type ActorEventFilter struct {
	// Matches events from one of these actors, or any actor if empty.
	// For now, this MUST be a Filecoin address.
	Addresses []address.Address `json:"address"`

	// Matches events with the specified key/values, or all events if empty.
	// If the `Blocks` slice is empty, matches on the key only.
	Fields map[string][]ActorEventBlock `json:"fields"`

	// Interpreted as an epoch (in hex) or one of "latest" for last mined block, "earliest" for first,
	// Optional, default: "latest".
	FromBlock *string `json:"fromBlock,omitempty"`

	// Interpreted as an epoch (in hex) or one of "latest" for last mined block, "earliest" for first,
	// Optional, default: "latest".
	ToBlock *string `json:"toBlock,omitempty"`

	// Restricts events returned to those emitted from messages contained in this tipset.
	// If `TipSetKey` is present in the filter criteria, then neither `FromBlock` nor `ToBlock` are allowed.
	TipSetKey *cid.Cid `json:"tipset_cid,omitempty"`
}

type ActorEvent struct {
	// event logs
	Entries []EventEntry `json:"entries"`

	// filecoin address of the actor that emitted this event
	EmitterAddr address.Address `json:"emitter"`

	// reverted is set to true if the message that produced this event was reverted because of a network re-org
	// in that case, the event should be considered as reverted as well
	Reverted bool `json:"reverted"`

	// height of the tipset that contained the message that produced this event
	Height abi.ChainEpoch `json:"height"`

	// cid of the tipset that contained the message that produced this event
	TipSetKey cid.Cid `json:"tipset_cid"`

	// cid of message that produced this event
	MsgCid cid.Cid `json:"msg_cid"`
}
