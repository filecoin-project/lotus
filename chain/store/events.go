package store

import (
	"bytes"
	"context"
	"unsafe"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	amt4 "github.com/filecoin-project/go-amt-ipld/v4"

	"github.com/filecoin-project/lotus/chain/types"
)

// ReadEvents loads the events under an event AMT root, as referenced by
// MessageReceipt.EventsRoot. Event AMTs are dense, so entries must occupy
// indices 0 to Len()-1 in order. A non-zero maxBytes bounds the decoded events'
// estimated size: encoded size plus struct overhead.
func ReadEvents(ctx context.Context, cst cbor.IpldStore, root cid.Cid, maxBytes uint64) ([]types.Event, error) {
	arr, err := amt4.LoadAMT(ctx, cst, root, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
	if err != nil {
		return nil, xerrors.Errorf("load events amt: %w", err)
	}

	count := arr.Len()
	events := []types.Event{}
	var size uint64
	err = arr.ForEach(ctx, func(i uint64, deferred *cbg.Deferred) error {
		if i != uint64(len(events)) || i >= count {
			return xerrors.Errorf("unexpected event index %d, expected %d of %d", i, len(events), count)
		}
		size += uint64(len(deferred.Raw))
		if maxBytes > 0 && size > maxBytes {
			return xerrors.Errorf("events exceed the %d byte limit", maxBytes)
		}
		var evt types.Event
		if err := evt.UnmarshalCBOR(bytes.NewReader(deferred.Raw)); err != nil {
			return xerrors.Errorf("decode event %d: %w", i, err)
		}
		size += uint64(unsafe.Sizeof(evt)) + uint64(len(evt.Entries))*uint64(unsafe.Sizeof(types.EventEntry{}))
		if maxBytes > 0 && size > maxBytes {
			return xerrors.Errorf("events exceed the %d byte limit", maxBytes)
		}
		events = append(events, evt)
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("read events amt: %w", err)
	}
	if uint64(len(events)) != count {
		return nil, xerrors.Errorf("events amt has %d entries, expected %d", len(events), count)
	}

	return events, nil
}
