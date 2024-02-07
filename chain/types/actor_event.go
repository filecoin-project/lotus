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
	// If `Prefill` is true and `FromEpoch` is set to latest, the pre-fill operation will become a no-op.
	// if `Prefill` is false and `FromEpoch` is set to earliest, historical events will still be sent to the client.
	Prefill bool `json:"prefill"`
}

type ActorEventFilter struct {
	// Matches events from one of these actors, or any actor if empty.
	// For now, this MUST be a Filecoin address.
	Addresses []address.Address `json:"addresses"`

	// Matches events with the specified key/values, or all events if empty.
	// If the value is an empty slice, the filter will match on the key only, accepting any value.
	Fields map[string][]ActorEventBlock `json:"fields,omitempty"`

	// Interpreted as an epoch (in hex) or one of "latest" for last mined block, "earliest" for first,
	// Optional, default: "latest".
	FromEpoch string `json:"fromEpoch,omitempty"`

	// Interpreted as an epoch (in hex) or one of "latest" for last mined block, "earliest" for first,
	// Optional, default: "latest".
	ToEpoch string `json:"toEpoch,omitempty"`

	// Restricts events returned to those emitted from messages contained in this tipset.
	// If `TipSetCid` is present in the filter criteria, then neither `FromEpoch` nor `ToEpoch` are allowed.
	TipSetCid *cid.Cid `json:"tipsetCid,omitempty"`
}

type ActorEvent struct {
	Entries     []EventEntry
	EmitterAddr address.Address // f4 address of emitter
	Reverted    bool
	Height      abi.ChainEpoch
	TipSetKey   cid.Cid // tipset that contained the message
	MsgCid      cid.Cid // cid of message that produced event
}
