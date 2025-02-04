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

	// if we hit max results, we should get an error
	// we have total 4 events
	testCases := []struct {
		name          string
		maxResults    int
		expectedCount int
		expectedErr   string
	}{
		{name: "no max results", maxResults: 0, expectedCount: 4},
		{name: "max result more that total events", maxResults: 10, expectedCount: 4},
		{name: "max results less than total events", maxResults: 1, expectedErr: ErrMaxResultsReached.Error()},
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
