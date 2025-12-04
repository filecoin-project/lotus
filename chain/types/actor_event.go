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

type ActorEventFilter struct {
	// Matches events from one of these actors, or any actor if empty.
	// For now, this MUST be a Filecoin address.
	Addresses []address.Address `json:"addresses,omitempty"`

	// Matches events with the specified key/values, or all events if empty.
	// If the value is an empty slice, the filter will match on the key only, accepting any value.
	Fields map[string][]ActorEventBlock `json:"fields,omitempty"`

	// The height of the earliest tipset to include in the query. If empty, the query starts at the
	// last finalized tipset.
	// NOTE: In a future upgrade, this will be strict when set and will result in an error if a filter
	// cannot be fulfilled by the depth of history available in the node. Currently, the node will
	// not return an error, but will return starting from the epoch it has data for.
	FromHeight *abi.ChainEpoch `json:"fromHeight,omitempty"`

	// The height of the latest tipset to include in the query. If empty, the query ends at the
	// latest tipset.
	ToHeight *abi.ChainEpoch `json:"toHeight,omitempty"`

	// Restricts events returned to those emitted from messages contained in this tipset.
	// If `TipSetKey` is left empty in the filter criteria, then neither `FromHeight` nor `ToHeight` are allowed.
	TipSetKey *TipSetKey `json:"tipsetKey,omitempty"`
}

type ActorEvent struct {
	// Event entries in log form.
	Entries []EventEntry `json:"entries"`

	// Filecoin address of the actor that emitted this event.
	// NOTE: In a future upgrade, this will change to always be an ID address. Currently this will be
	// either the f4 address, or ID address if an f4 is not available for this actor.
	Emitter address.Address `json:"emitter"`

	// Reverted is set to true if the message that produced this event was reverted because of a network re-org
	// in that case, the event should be considered as reverted as well.
	Reverted bool `json:"reverted"`

	// Height of the tipset that contained the message that produced this event.
	Height abi.ChainEpoch `json:"height"`

	// The tipset that contained the message that produced this event.
	TipSetKey TipSetKey `json:"tipsetKey"`

	// CID of message that produced this event.
	MsgCid cid.Cid `json:"msgCid"`
}
