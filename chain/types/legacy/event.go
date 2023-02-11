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

type EventEntry struct {
	// A bitmap conveying metadata or hints about this entry.
	Flags uint8

	// The key of this event entry
	Key string

	// The event value
	Value []byte
}

// Adapt method assumes that all events are EVM events (which is the case for
// nv<20, the network versions for which this code is active), and performs the
// following adaptations:
// - Upgrades the schema to new Events, setting codec = Raw.
// - Removes the CBOR framing from values.
// - Left pads EVM log topic entry values to 32 bytes.
func (e *Event) Adapt() (types.Event, error) {
	entries := make([]types.EventEntry, 0, len(e.Entries))
	for _, ee := range e.Entries {
		entry := types.EventEntry{
			Flags: ee.Flags,
			Key:   ee.Key,
			Codec: uint64(multicodec.Raw),
			Value: ee.Value,
		}
		value, err := cbg.ReadByteArray(bytes.NewReader(ee.Value), 64)
		if err != nil {
			return types.Event{}, fmt.Errorf("failed to decode event value while adapting: %w", err)
		}
		if l := len(value); l < 32 {
			pvalue := make([]byte, 32)
			copy(pvalue[32-len(value):], value)
			value = pvalue
		}
		entries = append(entries, entry)
	}
	return types.Event{
		Emitter: e.Emitter,
		Entries: entries,
	}, nil
}
