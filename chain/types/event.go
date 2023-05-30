package types

import (
	"bytes"
	"fmt"

	cbg "github.com/whyrusleeping/cbor-gen"

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

	// The event value
	Value []byte
}

type FilterID [32]byte // compatible with EthHash

// DecodeEvents decodes a CBOR list of CBOR-encoded events.
func DecodeEvents(input []byte) ([]Event, error) {
	r := bytes.NewReader(input)
	typ, len, err := cbg.NewCborReader(r).ReadHeader()
	if err != nil {
		return nil, fmt.Errorf("failed to read events: %w", err)
	}
	if typ != cbg.MajArray {
		return nil, fmt.Errorf("expected a CBOR list, was major type %d", typ)
	}
	events := make([]Event, 0, len)
	for i := 0; i < int(len); i++ {
		var evt Event
		if err := evt.UnmarshalCBOR(r); err != nil {
			return nil, fmt.Errorf("failed to parse event: %w", err)
		}
		events = append(events, evt)
	}
	return events, nil
}
