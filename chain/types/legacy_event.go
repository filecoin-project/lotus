package types

import (
	"bytes"
	"fmt"

	"github.com/multiformats/go-multicodec"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/abi"
)

type LegacyEvent struct {
	// The ID of the actor that emitted this event.
	Emitter abi.ActorID

	// Key values making up this event.
	Entries []LegacyEventEntry
}

type LegacyEventEntry struct {
	// A bitmap conveying metadata or hints about this entry.
	Flags uint8

	// The key of this event entry
	Key string

	// The event value
	Value []byte
}

var keyRewrites = map[string]string{
	"topic1": "t1",
	"topic2": "t2",
	"topic3": "t3",
	"topic4": "t4",
	"data":   "d",
}

// Adapt method assumes that all events are EVM events (which is the case for
// nv<20, the network versions for which this code is active), and performs the
// following adaptations:
// - Upgrades the schema to new Events, setting codec = Raw.
// - Removes the CBOR framing from values.
// - Left pads EVM log topic entry values to 32 bytes.
func (e *LegacyEvent) Adapt() (Event, error) {
	entries := make([]EventEntry, 0, len(e.Entries))
	for _, ee := range e.Entries {
		entry := EventEntry{
			Flags: ee.Flags,
			Key:   ee.Key,
			Codec: uint64(multicodec.Raw),
			Value: ee.Value,
		}
		var err error
		entry.Value, err = cbg.ReadByteArray(bytes.NewReader(ee.Value), 64)
		if err != nil {
			return Event{}, fmt.Errorf("failed to decode event value while adapting: %w", err)
		}
		if l := len(entry.Value); l < 32 {
			pvalue := make([]byte, 32)
			copy(pvalue[32-len(entry.Value):], entry.Value)
			entry.Value = pvalue
		}
		if r, ok := keyRewrites[entry.Key]; ok {
			entry.Key = r
		}
		entries = append(entries, entry)
	}
	return Event{
		Emitter: e.Emitter,
		Entries: entries,
	}, nil
}

// AsLegacy strips the codec off an Event object and returns a LegacyEvent.
func (e *Event) AsLegacy() *LegacyEvent {
	entries := make([]LegacyEventEntry, 0, len(e.Entries))
	for _, ee := range e.Entries {
		entry := LegacyEventEntry{
			Flags: ee.Flags,
			Key:   ee.Key,
			Value: ee.Value,
		}
		entries = append(entries, entry)
	}
	return &LegacyEvent{
		Emitter: e.Emitter,
		Entries: entries,
	}
}
