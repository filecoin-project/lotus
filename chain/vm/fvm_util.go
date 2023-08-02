package vm

import (
	"bytes"
	"fmt"

	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/chain/types"
)

const MaxEventSliceLength = 6_000_000

// DecodeEvents decodes a CBOR list of CBOR-encoded events.
func decodeEvents(input []byte) ([]types.Event, error) {
	r := bytes.NewReader(input)
	typ, length, err := cbg.NewCborReader(r).ReadHeader()
	if err != nil {
		return nil, fmt.Errorf("failed to read events: %w", err)
	}

	if length > MaxEventSliceLength {
		log.Errorf("extremely long event slice (len %d) returned, not decoding", length)
		return nil, nil
	}

	if typ != cbg.MajArray {
		return nil, fmt.Errorf("expected a CBOR list, was major type %d", typ)
	}
	events := make([]types.Event, 0, length)
	for i := 0; i < int(length); i++ {
		var evt types.Event
		if err := evt.UnmarshalCBOR(r); err != nil {
			return nil, fmt.Errorf("failed to parse event: %w", err)
		}
		events = append(events, evt)
	}
	return events, nil
}
