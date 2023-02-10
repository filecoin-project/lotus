package legacy

import (
	"bytes"
	"fmt"

	"github.com/multiformats/go-multicodec"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

type Event struct {
	// The ID of the actor that emitted this event.
	Emitter abi.ActorID

	// Key values making up this event.
	Entries []EventEntry
}

func (e *Event) Migrate() types.Event {
	entries := make([]types.EventEntry, 0, len(e.Entries))
	for _, ee := range e.Entries {
		entries = append(entries, types.EventEntry{
			Flags: ee.Flags,
			Key:   ee.Key,
			Codec: uint64(multicodec.Raw),
			Value: ee.Value,
		})
	}
	return types.Event{
		Emitter: e.Emitter,
		Entries: entries,
	}
}

type EventEntry struct {
	// A bitmap conveying metadata or hints about this entry.
	Flags uint8

	// The key of this event entry
	Key string

	// The event value
	Value []byte
}

// DecodeEvents decodes legacy events and translates them into new events.
func DecodeEvents(input []byte) ([]types.Event, error) {
	r := bytes.NewReader(input)
	typ, len, err := cbg.NewCborReader(r).ReadHeader()
	if err != nil {
		return nil, fmt.Errorf("failed to read events: %w", err)
	}
	if typ != cbg.MajArray {
		return nil, fmt.Errorf("expected a CBOR list, was major type %d", typ)
	}

	events := make([]types.Event, 0, len)
	for i := 0; i < int(len); i++ {
		var evt Event
		if err := evt.UnmarshalCBOR(r); err != nil {
			return nil, fmt.Errorf("failed to parse event: %w", err)
		}
		events = append(events, evt.Migrate())
	}
	return events, nil
}
