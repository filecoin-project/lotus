package types

import (
	"github.com/filecoin-project/go-state-types/abi"
)

// EventEntry flags defined in fvm_shared
const (
	EventFlagIndexedKey   = 0b00000001
	EventFlagIndexedValue = 0b00000010
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

	// The event value's codec
	Codec uint64

	// The event value. It is encoded using the codec specified above
	Value []byte
}

type FilterID [32]byte // compatible with EthHash
