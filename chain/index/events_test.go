package index

import (
	"context"
	"database/sql"
	"errors"
	pseudo "math/rand"
	"sort"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/must"
)

func TestGetEventsForFilterNoEvents(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))

	headHeight := abi.ChainEpoch(60)
	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })

	// Create a fake tipset at various heights used in the test
	fakeTipsets := make(map[abi.ChainEpoch]*types.TipSet)
	for _, ts := range []abi.ChainEpoch{1, 10, 20} {
		fakeTipsets[ts] = fakeTipSet(t, rng, ts, nil)
		cs.SetTipsetByHeightAndKey(ts, fakeTipsets[ts].Key(), fakeTipsets[ts])
		cs.SetTipSetByCid(t, fakeTipsets[ts])
	}

	// tipset is not indexed
	f := &EventFilter{
		MinHeight: 1,
		MaxHeight: 1,
	}
	ces, err := si.GetEventsForFilter(ctx, f)
	require.True(t, errors.Is(err, ErrNotFound))
	require.Equal(t, 0, len(ces))

	tsCid, err := fakeTipsets[1].Key().Cid()
	require.NoError(t, err)
	f = &EventFilter{
		TipsetCid: tsCid,
	}

	ces, err = si.GetEventsForFilter(ctx, f)
	require.True(t, errors.Is(err, ErrNotFound))
	require.Equal(t, 0, len(ces))

	// tipset is indexed but has no events
	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		return si.indexTipset(ctx, tx, fakeTipsets[1])
	})
	require.NoError(t, err)

	ces, err = si.GetEventsForFilter(ctx, f)
	require.NoError(t, err)
	require.Equal(t, 0, len(ces))

	f = &EventFilter{
		TipsetCid: tsCid,
	}
	ces, err = si.GetEventsForFilter(ctx, f)
	require.NoError(t, err)
	require.Equal(t, 0, len(ces))

	// search for a range that is not indexed
	f = &EventFilter{
		MinHeight: 10,
		MaxHeight: 20,
	}
	ces, err = si.GetEventsForFilter(ctx, f)
	require.ErrorIs(t, err, ErrNotFound)
	require.Equal(t, 0, len(ces))

	// search for a range (end) that is in the future
	f = &EventFilter{
		MinHeight: 10,
		MaxHeight: 200,
	}
	ces, err = si.GetEventsForFilter(ctx, f)
	require.ErrorIs(t, err, &ErrRangeInFuture{})
	require.Equal(t, 0, len(ces))

	// search for a range (start too) that is in the future
	f = &EventFilter{
		MinHeight: 100,
		MaxHeight: 200,
	}
	ces, err = si.GetEventsForFilter(ctx, f)
	require.ErrorIs(t, err, &ErrRangeInFuture{})
	require.Equal(t, 0, len(ces))
}

func TestGetEventsForFilterWithEvents(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(60)
	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })

	ev1 := fakeEvent(
		abi.ActorID(1),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr1")},
		},
		[]kv{
			{k: "amount", v: []byte("2988181")},
		},
	)

	ev2 := fakeEvent(
		abi.ActorID(2),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr2")},
		},
		[]kv{
			{k: "amount", v: []byte("2988181")},
		},
	)

	events := []types.Event{*ev1, *ev2}

	fm := fakeMessage(address.TestAddress, address.TestAddress)
	em1 := executedMessage{
		msg: fm,
		evs: events,
	}

	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}

		return idAddr, true
	})

	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return []executedMessage{em1}, nil
	})

	// Create a fake tipset at height 1
	fakeTipSet1 := fakeTipSet(t, rng, 1, nil)
	fakeTipSet2 := fakeTipSet(t, rng, 2, nil)

	// Set the dummy chainstore to return this tipset for height 1
	cs.SetTipsetByHeightAndKey(1, fakeTipSet1.Key(), fakeTipSet1) // empty DB
	cs.SetTipsetByHeightAndKey(2, fakeTipSet2.Key(), fakeTipSet2) // empty DB
	cs.SetTipSetByCid(t, fakeTipSet1)
	cs.SetTipSetByCid(t, fakeTipSet2)

	cs.SetMessagesForTipset(fakeTipSet1, []types.ChainMsg{fm})

	// index tipset and events
	require.NoError(t, si.Apply(ctx, fakeTipSet1, fakeTipSet2))

	// fetch it based on height -> works
	f := &EventFilter{
		MinHeight: 1,
		MaxHeight: 1,
	}
	ces, err := si.GetEventsForFilter(ctx, f)
	require.NoError(t, err)
	require.Equal(t, 2, len(ces))

	// fetch it based on cid -> works
	tsCid1, err := fakeTipSet1.Key().Cid()
	require.NoError(t, err)

	tsCid2, err := fakeTipSet2.Key().Cid()
	require.NoError(t, err)

	f = &EventFilter{
		TipsetCid: tsCid1,
	}
	ces, err = si.GetEventsForFilter(ctx, f)
	require.NoError(t, err)

	require.Equal(t, []*CollectedEvent{
		{
			Entries:     ev1.Entries,
			EmitterAddr: must.One(address.NewIDAddress(uint64(ev1.Emitter))),
			EventIdx:    0,
			Reverted:    false,
			Height:      1,
			TipSetKey:   fakeTipSet1.Key(),
			MsgIdx:      0,
			MsgCid:      fm.Cid(),
		},
		{
			Entries:     ev2.Entries,
			EmitterAddr: must.One(address.NewIDAddress(uint64(ev2.Emitter))),
			EventIdx:    1,
			Reverted:    false,
			Height:      1,
			TipSetKey:   fakeTipSet1.Key(),
			MsgIdx:      0,
			MsgCid:      fm.Cid(),
		},
	}, ces)

	// mark fakeTipSet2 as reverted so events for fakeTipSet1 are reverted
	require.NoError(t, si.Revert(ctx, fakeTipSet2, fakeTipSet1))

	var reverted bool
	err = si.db.QueryRow("SELECT reverted FROM tipset_message WHERE tipset_key_cid = ?", tsCid2.Bytes()).Scan(&reverted)
	require.NoError(t, err)
	require.True(t, reverted)

	var reverted2 bool
	err = si.db.QueryRow("SELECT reverted FROM tipset_message WHERE tipset_key_cid = ?", tsCid1.Bytes()).Scan(&reverted2)
	require.NoError(t, err)
	require.False(t, reverted2)

	// fetching events fails if excludeReverted is true i.e. we request events by height
	f = &EventFilter{
		MinHeight: 1,
		MaxHeight: 1,
	}
	ces, err = si.GetEventsForFilter(ctx, f)
	require.NoError(t, err)
	require.Equal(t, 0, len(ces))

	// works if excludeReverted is false i.e. we request events by hash
	f = &EventFilter{
		TipsetCid: tsCid1,
	}
	ces, err = si.GetEventsForFilter(ctx, f)
	require.NoError(t, err)
	require.Equal(t, 2, len(ces))
}

func TestGetEventsFilterByAddress(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(60)
	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })

	addr1, err := address.NewIDAddress(1)
	require.NoError(t, err)
	addr2, err := address.NewIDAddress(2)
	require.NoError(t, err)
	addr3, err := address.NewIDAddress(3)
	require.NoError(t, err)

	delegatedAddr1, err := address.NewFromString("f410fagkp3qx2f76maqot74jaiw3tzbxe76k76zrkl3xifk67isrnbn2sll3yua")
	require.NoError(t, err)

	ev1 := fakeEvent(
		abi.ActorID(1),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr1")},
		},
		[]kv{
			{k: "amount", v: []byte("2988181")},
		},
	)

	ev2 := fakeEvent(
		abi.ActorID(2),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr2")},
		},
		[]kv{
			{k: "amount", v: []byte("2988181")},
		},
	)

	events := []types.Event{*ev1, *ev2}

	fm := fakeMessage(address.TestAddress, address.TestAddress)
	em1 := executedMessage{
		msg: fm,
		evs: events,
	}

	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		if emitter == abi.ActorID(1) {
			return delegatedAddr1, true
		}
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}
		return idAddr, true
	})

	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return []executedMessage{em1}, nil
	})

	// Create a fake tipset at height 1
	fakeTipSet1 := fakeTipSet(t, rng, 1, nil)
	fakeTipSet2 := fakeTipSet(t, rng, 2, nil)

	// Set the dummy chainstore to return this tipset for height 1
	cs.SetTipsetByHeightAndKey(1, fakeTipSet1.Key(), fakeTipSet1) // empty DB
	cs.SetTipsetByHeightAndKey(2, fakeTipSet2.Key(), fakeTipSet2) // empty DB
	cs.SetTipSetByCid(t, fakeTipSet1)
	cs.SetTipSetByCid(t, fakeTipSet2)

	cs.SetMessagesForTipset(fakeTipSet1, []types.ChainMsg{fm})

	require.NoError(t, si.Apply(ctx, fakeTipSet1, fakeTipSet2))

	testCases := []struct {
		name              string
		f                 *EventFilter
		expectedCount     int
		expectedAddresses []address.Address
	}{
		{
			name: "matching single ID address (non-delegated)",
			f: &EventFilter{
				Addresses: []address.Address{addr2},
				MinHeight: 1,
				MaxHeight: 1,
			},
			expectedCount:     1,
			expectedAddresses: []address.Address{addr2},
		},
		{
			name: "matching single ID address",
			f: &EventFilter{
				Addresses: []address.Address{addr1},
				MinHeight: 1,
				MaxHeight: 1,
			},
			expectedCount:     1,
			expectedAddresses: []address.Address{delegatedAddr1},
		},
		{
			name: "matching single delegated address",
			f: &EventFilter{
				Addresses: []address.Address{delegatedAddr1},
				MinHeight: 1,
				MaxHeight: 1,
			},
			expectedCount:     1,
			expectedAddresses: []address.Address{delegatedAddr1},
		},
		{
			name: "matching multiple addresses",
			f: &EventFilter{
				Addresses: []address.Address{addr1, addr2},
				MinHeight: 1,
				MaxHeight: 1,
			},
			expectedCount:     2,
			expectedAddresses: []address.Address{delegatedAddr1, addr2},
		},
		{
			name: "no matching address",
			f: &EventFilter{
				Addresses: []address.Address{addr3},
				MinHeight: 1,
				MaxHeight: 1,
			},
			expectedCount:     0,
			expectedAddresses: []address.Address{},
		},
		{
			name: "empty address list",
			f: &EventFilter{
				Addresses: []address.Address{},
				MinHeight: 1,
				MaxHeight: 1,
			},
			expectedCount:     2,
			expectedAddresses: []address.Address{delegatedAddr1, addr2},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ces, err := si.GetEventsForFilter(ctx, tc.f)
			require.NoError(t, err)
			require.Equal(t, tc.expectedCount, len(ces))

			actualAddresses := make([]address.Address, len(ces))
			for i, ce := range ces {
				actualAddresses[i] = ce.EmitterAddr
			}

			sortAddresses(tc.expectedAddresses)
			sortAddresses(actualAddresses)

			require.Equal(t, tc.expectedAddresses, actualAddresses)
		})
	}
}

func TestGetEventsForFilterWithRawCodec(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(60)

	// Setup the indexer and chain store with the specified head height
	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })

	// Define codec constants (replace with actual multicodec values)
	var (
		codecRaw  = multicodec.Raw
		codecCBOR = multicodec.Cbor
	)

	// Create events with different codecs
	evRaw1 := fakeEventWithCodec(
		abi.ActorID(1),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr1")},
		},
		codecRaw,
	)

	evCBOR := fakeEventWithCodec(
		abi.ActorID(2),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr2")},
		},
		codecCBOR,
	)

	evRaw2 := fakeEventWithCodec(
		abi.ActorID(3),
		[]kv{
			{k: "type", v: []byte("transfer")},
			{k: "recipient", v: []byte("addr3")},
		},
		codecRaw,
	)

	// Aggregate events
	events := []types.Event{*evRaw1, *evCBOR, *evRaw2}

	// Create a fake message and associate it with the events
	fm := fakeMessage(address.TestAddress, address.TestAddress)
	em1 := executedMessage{
		msg: fm,
		evs: events,
	}

	// Mock the Actor to Address mapping
	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}
		return idAddr, true
	})

	// Mock the executed messages loader
	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return []executedMessage{em1}, nil
	})

	// Create fake tipsets
	fakeTipSet1 := fakeTipSet(t, rng, 1, nil)
	fakeTipSet2 := fakeTipSet(t, rng, 2, nil)

	// Associate tipsets with their heights and CIDs
	cs.SetTipsetByHeightAndKey(1, fakeTipSet1.Key(), fakeTipSet1) // Height 1
	cs.SetTipsetByHeightAndKey(2, fakeTipSet2.Key(), fakeTipSet2) // Height 2
	cs.SetTipSetByCid(t, fakeTipSet1)
	cs.SetTipSetByCid(t, fakeTipSet2)

	// Associate messages with tipsets
	cs.SetMessagesForTipset(fakeTipSet1, []types.ChainMsg{fm})

	// Apply the indexer to process the tipsets
	require.NoError(t, si.Apply(ctx, fakeTipSet1, fakeTipSet2))

	t.Run("FilterEventsByRawCodecWithoutKeys", func(t *testing.T) {
		f := &EventFilter{
			MinHeight: 1,
			MaxHeight: 2,
			Codec:     codecRaw, // Set to RAW codec
		}

		// Retrieve events based on the filter
		ces, err := si.GetEventsForFilter(ctx, f)
		require.NoError(t, err)

		// Define expected collected events (only RAW encoded events)
		expectedCES := []*CollectedEvent{
			{
				Entries:     evRaw1.Entries,
				EmitterAddr: must.One(address.NewIDAddress(uint64(evRaw1.Emitter))),
				EventIdx:    0,
				Reverted:    false,
				Height:      1,
				TipSetKey:   fakeTipSet1.Key(),
				MsgIdx:      0,
				MsgCid:      fm.Cid(),
			},
			{
				Entries:     evRaw2.Entries,
				EmitterAddr: must.One(address.NewIDAddress(uint64(evRaw2.Emitter))),
				EventIdx:    2, // Adjust based on actual indexing
				Reverted:    false,
				Height:      1,
				TipSetKey:   fakeTipSet1.Key(),
				MsgIdx:      0,
				MsgCid:      fm.Cid(),
			},
		}

		require.Equal(t, expectedCES, ces)
	})
}

func TestMaxFilterResults(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(60)

	// Setup the indexer and chain store with the specified head height
	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })

	// Define codec constants (replace with actual multicodec values)
	var (
		codecRaw  = multicodec.Raw
		codecCBOR = multicodec.Cbor
	)

	// Create events with different codecs
	evRaw1 := fakeEventWithCodec(
		abi.ActorID(1),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr1")},
		},
		codecRaw,
	)

	evCBOR := fakeEventWithCodec(
		abi.ActorID(2),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr2")},
		},
		codecCBOR,
	)

	evRaw2 := fakeEventWithCodec(
		abi.ActorID(3),
		[]kv{
			{k: "type", v: []byte("transfer")},
			{k: "recipient", v: []byte("addr3")},
		},
		codecRaw,
	)

	evRaw3 := fakeEventWithCodec(
		abi.ActorID(4),
		[]kv{
			{k: "type", v: []byte("transfer")},
			{k: "recipient", v: []byte("addr4")},
		},
		codecCBOR,
	)

	// Aggregate events
	events := []types.Event{*evRaw1, *evCBOR, *evRaw2, *evRaw3}

	// Create a fake message and associate it with the events
	fm := fakeMessage(address.TestAddress, address.TestAddress)
	em1 := executedMessage{
		msg: fm,
		evs: events,
	}

	// Mock the Actor to Address mapping
	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}
		return idAddr, true
	})

	// Mock the executed messages loader
	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return []executedMessage{em1}, nil
	})

	// Create fake tipsets
	fakeTipSet1 := fakeTipSet(t, rng, 1, nil)
	fakeTipSet2 := fakeTipSet(t, rng, 2, nil)

	// Associate tipsets with their heights and CIDs
	cs.SetTipsetByHeightAndKey(1, fakeTipSet1.Key(), fakeTipSet1) // Height 1
	cs.SetTipsetByHeightAndKey(2, fakeTipSet2.Key(), fakeTipSet2) // Height 2
	cs.SetTipSetByCid(t, fakeTipSet1)
	cs.SetTipSetByCid(t, fakeTipSet2)

	// Associate messages with tipsets
	cs.SetMessagesForTipset(fakeTipSet1, []types.ChainMsg{fm})

	// Apply the indexer to process the tipsets
	require.NoError(t, si.Apply(ctx, fakeTipSet1, fakeTipSet2))

	// All 4 events are in one tipset; MaxResults applies at tipset boundaries so no bisection occurs.
	testCases := []struct {
		name          string
		maxResults    int
		expectedCount int
		expectedErr   string
	}{
		{name: "no max results", maxResults: 0, expectedCount: 4},
		{name: "max above total events", maxResults: 10, expectedCount: 4},
		{name: "max below tipset event count returns full tipset", maxResults: 1, expectedCount: 4},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := &EventFilter{
				MinHeight:  1,
				MaxHeight:  2,
				MaxResults: tc.maxResults,
			}

			ces, err := si.GetEventsForFilter(ctx, f)
			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedCount, len(ces))
			}
		})
	}
}

// TipsetCid-scoped queries return every event for the tipset regardless of MaxResults;
// eth_getTransactionReceipt and eth_getLogs-by-block-hash depend on this.
func TestMaxFilterResultsTipsetCidBypass(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(60)

	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })

	events := []types.Event{
		*fakeEventWithCodec(abi.ActorID(1), []kv{{k: "type", v: []byte("a")}}, multicodec.Raw),
		*fakeEventWithCodec(abi.ActorID(2), []kv{{k: "type", v: []byte("b")}}, multicodec.Raw),
		*fakeEventWithCodec(abi.ActorID(3), []kv{{k: "type", v: []byte("c")}}, multicodec.Raw),
		*fakeEventWithCodec(abi.ActorID(4), []kv{{k: "type", v: []byte("d")}}, multicodec.Raw),
	}

	fm := fakeMessage(address.TestAddress, address.TestAddress)
	em1 := executedMessage{msg: fm, evs: events}

	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}
		return idAddr, true
	})
	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return []executedMessage{em1}, nil
	})

	fakeTipSet1 := fakeTipSet(t, rng, 1, nil)
	fakeTipSet2 := fakeTipSet(t, rng, 2, nil)
	cs.SetTipsetByHeightAndKey(1, fakeTipSet1.Key(), fakeTipSet1)
	cs.SetTipsetByHeightAndKey(2, fakeTipSet2.Key(), fakeTipSet2)
	cs.SetTipSetByCid(t, fakeTipSet1)
	cs.SetTipSetByCid(t, fakeTipSet2)
	cs.SetMessagesForTipset(fakeTipSet1, []types.ChainMsg{fm})
	require.NoError(t, si.Apply(ctx, fakeTipSet1, fakeTipSet2))

	tsCid, err := fakeTipSet1.Key().Cid()
	require.NoError(t, err)

	f := &EventFilter{
		TipsetCid:  tsCid,
		MaxResults: 1,
	}
	ces, err := si.GetEventsForFilter(ctx, f)
	require.NoError(t, err)
	require.Equal(t, 4, len(ces))
}

// MaxResults is a hard cap once a range query's events span more than one tipset; a
// single contributing tipset may exceed MaxResults but two or more tipsets together
// must not.
func TestMaxFilterResultsTipsetBoundary(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(60)

	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })

	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}
		return idAddr, true
	})

	// Two events at height 1, two events at height 2.
	fm1 := fakeMessage(address.TestAddress, address.TestAddress)
	fm2 := fakeMessage(address.TestAddress, address.TestAddress)
	fm2.Nonce = 198
	em1 := executedMessage{
		msg: fm1,
		evs: []types.Event{
			*fakeEventWithCodec(abi.ActorID(1), []kv{{k: "type", v: []byte("a")}}, multicodec.Raw),
			*fakeEventWithCodec(abi.ActorID(2), []kv{{k: "type", v: []byte("b")}}, multicodec.Raw),
		},
	}
	em2 := executedMessage{
		msg: fm2,
		evs: []types.Event{
			*fakeEventWithCodec(abi.ActorID(3), []kv{{k: "type", v: []byte("c")}}, multicodec.Raw),
			*fakeEventWithCodec(abi.ActorID(4), []kv{{k: "type", v: []byte("d")}}, multicodec.Raw),
		},
	}

	fakeTipSet1 := fakeTipSet(t, rng, 1, nil)
	fakeTipSet2 := fakeTipSet(t, rng, 2, nil)
	fakeTipSet3 := fakeTipSet(t, rng, 3, nil)
	cs.SetTipsetByHeightAndKey(1, fakeTipSet1.Key(), fakeTipSet1)
	cs.SetTipsetByHeightAndKey(2, fakeTipSet2.Key(), fakeTipSet2)
	cs.SetTipsetByHeightAndKey(3, fakeTipSet3.Key(), fakeTipSet3)
	cs.SetTipSetByCid(t, fakeTipSet1)
	cs.SetTipSetByCid(t, fakeTipSet2)
	cs.SetTipSetByCid(t, fakeTipSet3)
	cs.SetMessagesForTipset(fakeTipSet1, []types.ChainMsg{fm1})
	cs.SetMessagesForTipset(fakeTipSet2, []types.ChainMsg{fm2})

	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return []executedMessage{em1}, nil
	})
	require.NoError(t, si.Apply(ctx, fakeTipSet1, fakeTipSet2))

	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return []executedMessage{em2}, nil
	})
	require.NoError(t, si.Apply(ctx, fakeTipSet2, fakeTipSet3))

	testCases := []struct {
		name          string
		maxResults    int
		expectedCount int
		expectedErr   string
	}{
		{name: "no max results returns all", maxResults: 0, expectedCount: 4},
		{name: "max above total returns all", maxResults: 10, expectedCount: 4},
		{name: "max equal to total returns all", maxResults: 4, expectedCount: 4},
		{name: "max below total fires", maxResults: 3, expectedErr: ErrMaxResultsReached.Error()},
		{name: "max equal to first tipset count fires", maxResults: 2, expectedErr: ErrMaxResultsReached.Error()},
		{name: "max below first tipset count fires", maxResults: 1, expectedErr: ErrMaxResultsReached.Error()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := &EventFilter{
				MinHeight:  1,
				MaxHeight:  2,
				MaxResults: tc.maxResults,
			}

			ces, err := si.GetEventsForFilter(ctx, f)
			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedCount, len(ces))
			}
		})
	}
}

// MsgCid restricts filter queries to events emitted by a single message; this exercises
// per-tx scoping for eth_getTransactionReceipt against a tipset that contains other
// messages. The MsgCid path also bypasses MaxResults because the natural unit is the
// message's events.
func TestEventFilterMsgCid(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(60)

	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })

	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}
		return idAddr, true
	})

	// Two messages in the same tipset; each emits two events.
	fm1 := fakeMessage(address.TestAddress, address.TestAddress)
	fm2 := fakeMessage(address.TestAddress, address.TestAddress)
	fm2.Nonce = 198
	em1 := executedMessage{
		msg: fm1,
		evs: []types.Event{
			*fakeEventWithCodec(abi.ActorID(1), []kv{{k: "type", v: []byte("m1a")}}, multicodec.Raw),
			*fakeEventWithCodec(abi.ActorID(2), []kv{{k: "type", v: []byte("m1b")}}, multicodec.Raw),
		},
	}
	em2 := executedMessage{
		msg: fm2,
		evs: []types.Event{
			*fakeEventWithCodec(abi.ActorID(3), []kv{{k: "type", v: []byte("m2a")}}, multicodec.Raw),
			*fakeEventWithCodec(abi.ActorID(4), []kv{{k: "type", v: []byte("m2b")}}, multicodec.Raw),
		},
	}

	fakeTipSet1 := fakeTipSet(t, rng, 1, nil)
	fakeTipSet2 := fakeTipSet(t, rng, 2, nil)
	cs.SetTipsetByHeightAndKey(1, fakeTipSet1.Key(), fakeTipSet1)
	cs.SetTipsetByHeightAndKey(2, fakeTipSet2.Key(), fakeTipSet2)
	cs.SetTipSetByCid(t, fakeTipSet1)
	cs.SetTipSetByCid(t, fakeTipSet2)
	cs.SetMessagesForTipset(fakeTipSet1, []types.ChainMsg{fm1, fm2})

	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return []executedMessage{em1, em2}, nil
	})
	require.NoError(t, si.Apply(ctx, fakeTipSet1, fakeTipSet2))

	tsCid, err := fakeTipSet1.Key().Cid()
	require.NoError(t, err)
	otherTsCid, err := fakeTipSet2.Key().Cid()
	require.NoError(t, err)

	requireEventEmitter := func(t *testing.T, ces []*CollectedEvent, expectedEmitters ...abi.ActorID) {
		t.Helper()
		require.Equal(t, len(expectedEmitters), len(ces))
		for i, ce := range ces {
			expected, err := address.NewIDAddress(uint64(expectedEmitters[i]))
			require.NoError(t, err)
			require.Equal(t, expected, ce.EmitterAddr)
		}
	}

	t.Run("MsgCid scopes to single message", func(t *testing.T) {
		ces, err := si.GetEventsForFilter(ctx, &EventFilter{
			MinHeight: 1,
			MaxHeight: 1,
			MsgCid:    fm1.Cid(),
		})
		require.NoError(t, err)
		requireEventEmitter(t, ces, 1, 2)

		ces, err = si.GetEventsForFilter(ctx, &EventFilter{
			MinHeight: 1,
			MaxHeight: 1,
			MsgCid:    fm2.Cid(),
		})
		require.NoError(t, err)
		requireEventEmitter(t, ces, 3, 4)
	})

	t.Run("without MsgCid returns all messages' events", func(t *testing.T) {
		ces, err := si.GetEventsForFilter(ctx, &EventFilter{
			MinHeight: 1,
			MaxHeight: 1,
		})
		require.NoError(t, err)
		requireEventEmitter(t, ces, 1, 2, 3, 4)
	})

	t.Run("MsgCid combined with matching TipsetCid", func(t *testing.T) {
		ces, err := si.GetEventsForFilter(ctx, &EventFilter{
			TipsetCid: tsCid,
			MsgCid:    fm1.Cid(),
		})
		require.NoError(t, err)
		requireEventEmitter(t, ces, 1, 2)
	})

	t.Run("MsgCid combined with mismatched TipsetCid returns nothing", func(t *testing.T) {
		ces, err := si.GetEventsForFilter(ctx, &EventFilter{
			TipsetCid: otherTsCid,
			MsgCid:    fm1.Cid(),
		})
		require.NoError(t, err)
		require.Empty(t, ces)
	})

	t.Run("MsgCid not present in tipset returns nothing", func(t *testing.T) {
		// A valid CID the indexer has not seen as a message; the tipset is indexed so the
		// filter just returns no rows.
		ces, err := si.GetEventsForFilter(ctx, &EventFilter{
			MinHeight: 1,
			MaxHeight: 1,
			MsgCid:    tsCid,
		})
		require.NoError(t, err)
		require.Empty(t, ces)
	})

	t.Run("MsgCid bypasses MaxResults", func(t *testing.T) {
		ces, err := si.GetEventsForFilter(ctx, &EventFilter{
			MinHeight:  1,
			MaxHeight:  1,
			MsgCid:     fm1.Cid(),
			MaxResults: 1,
		})
		require.NoError(t, err)
		requireEventEmitter(t, ces, 1, 2)
	})
}

func sortAddresses(addrs []address.Address) {
	sort.Slice(addrs, func(i, j int) bool {
		return addrs[i].String() < addrs[j].String()
	})
}

func fakeMessage(to, from address.Address) *types.Message {
	return &types.Message{
		To:         to,
		From:       from,
		Nonce:      197,
		Method:     1,
		Params:     []byte("some random bytes"),
		GasLimit:   126723,
		GasPremium: types.NewInt(4),
		GasFeeCap:  types.NewInt(120),
	}
}

func fakeEvent(emitter abi.ActorID, indexed []kv, unindexed []kv) *types.Event {
	ev := &types.Event{
		Emitter: emitter,
	}

	for _, in := range indexed {
		ev.Entries = append(ev.Entries, types.EventEntry{
			Flags: 0x01,
			Key:   in.k,
			Codec: cid.Raw,
			Value: in.v,
		})
	}

	for _, in := range unindexed {
		ev.Entries = append(ev.Entries, types.EventEntry{
			Flags: 0x00,
			Key:   in.k,
			Codec: cid.Raw,
			Value: in.v,
		})
	}

	return ev
}

func fakeEventWithCodec(emitter abi.ActorID, indexed []kv, codec multicodec.Code) *types.Event {
	ev := &types.Event{
		Emitter: emitter,
	}

	for _, in := range indexed {
		ev.Entries = append(ev.Entries, types.EventEntry{
			Flags: 0x01,
			Key:   in.k,
			Codec: uint64(codec),
			Value: in.v,
		})
	}

	return ev
}

type kv struct {
	k string
	v []byte
}
