package types

import (
	"github.com/filecoin-project/go-state-types/abi"
)

type Event struct {
	// The ID of the actor that emitted this event.
	Emitter abi.ActorID

	// Key values making up this event.
	Entries []EventEntry
}

type EventEntry struct {
	// A bitmap conveying metadata or hints about this entry.
	Flags uint8

	// The key of this event entry
	Key string

	// Any DAG-CBOR encodeable type.
	Value []byte
}

type FilterID [16]byte

// EventEntry flags defined in fvm_shared
const (
	EventFlagIndexedKey   = 0b00000001
	EventFlagIndexedValue = 0b00000010
)
