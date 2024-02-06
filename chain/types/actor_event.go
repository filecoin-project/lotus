package types

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
)

type ActorEventBlock struct {
	// The value codec to match when filtering event values.
	Codec uint64 `json:"codec"`

	// The value to want to match on associated with the corresponding "event key"
	// when filtering events.
	// Should be a byte array encoded with the specified codec.
	// Assumes base64 encoding when converting to/from JSON strings.
	Value []byte `json:"value"`
}

type SubActorEventFilter struct {
	Filter ActorEventFilter `json:"filter"`

	// If true, all available matching historical events will be written to the response stream
	// before any new real-time events that match the given filter are written.
	Prefill bool `json:"prefill"`
}

type ActorEventFilter struct {
	// Matches events from one of these actors, or any actor if empty.
	// For now, this MUST be a Filecoin address.
	Addresses []address.Address `json:"address"`

	// Matches events with the specified key/values, or all events if empty.
	// If the value is an empty slice, the filter will match on the key only, accepting any value.
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
	// Event entries in log form.
	Entries []EventEntry `json:"entries"`

	// Filecoin address of the actor that emitted this event.
	EmitterAddr address.Address `json:"emitter"`

	// Reverted is set to true if the message that produced this event was reverted because of a network re-org
	// in that case, the event should be considered as reverted as well.
	Reverted bool `json:"reverted"`

	// Height of the tipset that contained the message that produced this event.
	Height abi.ChainEpoch `json:"height"`

	// CID of the tipset that contained the message that produced this event.
	TipSetKey cid.Cid `json:"tipset_cid"`

	// CID of message that produced this event.
	MsgCid cid.Cid `json:"msg_cid"`
}
