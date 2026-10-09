package store_test

import (
	"context"
	"math"
	"testing"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

const v4CountField = 2 // [bitWidth, height, count, node]

func testEvents(n int) []types.Event {
	events := make([]types.Event, n)
	for i := range events {
		events[i] = types.Event{
			Emitter: abi.ActorID(1000 + i),
			Entries: []types.EventEntry{{
				Flags: 0x03,
				Key:   "t1",
				Codec: cid.Raw,
				Value: []byte{byte(i)},
			}},
		}
	}
	return events
}

func putEvents(t *testing.T, cst cbor.IpldStore, events []types.Event) cid.Cid {
	vals := make([]cbg.CBORMarshaler, len(events))
	for i := range events {
		vals[i] = &events[i]
	}
	root, err := amt4.FromArray(context.Background(), cst, vals, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
	require.NoError(t, err)
	return root
}

func TestReadEvents(t *testing.T) {
	ctx := context.Background()
	bs := blockstore.NewMemory()
	cst := cbor.NewCborStore(bs)

	t.Run("round trip", func(t *testing.T) {
		// More than one leaf node at the event AMT bitwidth.
		events := testEvents(70)
		root := putEvents(t, cst, events)

		got, err := store.ReadEvents(ctx, cst, root, 0)
		require.NoError(t, err)
		require.Equal(t, events, got)

		got, err = store.ReadEvents(ctx, cst, root, 1<<20)
		require.NoError(t, err)
		require.Equal(t, events, got)
	})

	t.Run("size limit", func(t *testing.T) {
		_, err := store.ReadEvents(ctx, cst, putEvents(t, cst, testEvents(70)), 1<<10)
		require.ErrorContains(t, err, "events exceed")
	})

	t.Run("size limit charges struct overhead", func(t *testing.T) {
		events := make([]types.Event, 64) // each encodes to 3 bytes
		_, err := store.ReadEvents(ctx, cst, putEvents(t, cst, events), 64*3*2)
		require.ErrorContains(t, err, "events exceed")
	})

	t.Run("empty", func(t *testing.T) {
		got, err := store.ReadEvents(ctx, cst, putEvents(t, cst, nil), 0)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
	})

	t.Run("count above entries", func(t *testing.T) {
		root := setRootCount(t, bs, putEvents(t, cst, testEvents(10)), v4CountField, 11)
		_, err := store.ReadEvents(ctx, cst, root, 0)
		require.ErrorContains(t, err, "expected 11")
	})

	t.Run("count below entries", func(t *testing.T) {
		root := setRootCount(t, bs, putEvents(t, cst, testEvents(10)), v4CountField, 3)
		_, err := store.ReadEvents(ctx, cst, root, 0)
		require.ErrorContains(t, err, "unexpected event index 3")
	})

	t.Run("count at capacity of a tall root", func(t *testing.T) {
		// An entry at a high index makes the root tall enough for v4 to accept
		// any count.
		arr, err := amt4.NewAMT(cst, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
		require.NoError(t, err)
		evt := testEvents(1)[0]
		require.NoError(t, arr.Set(ctx, 1<<60, &evt))
		root, err := arr.Flush(ctx)
		require.NoError(t, err)

		root = setRootCount(t, bs, root, v4CountField, math.MaxUint64-1)
		_, err = amt4.LoadAMT(ctx, cst, root, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
		require.NoError(t, err)

		_, err = store.ReadEvents(ctx, cst, root, 0)
		require.ErrorContains(t, err, "unexpected event index")
	})

	t.Run("shared leaf nodes", func(t *testing.T) {
		// Identical events in every slot give identical leaf nodes, so both
		// links in the root point at one block.
		events := make([]types.Event, 64)
		for i := range events {
			events[i] = testEvents(1)[0]
		}
		root := putEvents(t, cst, events)
		got, err := store.ReadEvents(ctx, cst, root, 0)
		require.NoError(t, err)
		require.Len(t, got, 64)

		short := setRootCount(t, bs, root, v4CountField, 32)
		_, err = store.ReadEvents(ctx, cst, short, 0)
		require.ErrorContains(t, err, "unexpected event index 32")

		_, err = store.ReadEvents(ctx, cst, root, 1<<10)
		require.ErrorContains(t, err, "events exceed")
	})
}
