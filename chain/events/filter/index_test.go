package filter

import (
	"context"
	pseudo "math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestEventIndexPrefillFilter(t *testing.T) {
	rng := pseudo.New(pseudo.NewSource(299792458))
	a1 := randomF4Addr(t, rng)
	a2 := randomF4Addr(t, rng)

	a1ID := abi.ActorID(1)
	a2ID := abi.ActorID(2)

	addrMap := addressMap{}
	addrMap.add(a1ID, a1)
	addrMap.add(a2ID, a2)

	ev1 := fakeEvent(
		a1ID,
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr1")},
		},
		[]kv{
			{k: "amount", v: []byte("2988181")},
		},
	)

	st := newStore()
	events := []*types.Event{ev1}
	em := executedMessage{
		msg: fakeMessage(randomF4Addr(t, rng), randomF4Addr(t, rng)),
		rct: fakeReceipt(t, rng, st, events),
		evs: events,
	}

	events14000 := buildTipSetEvents(t, rng, 14000, em)
	cid14000, err := events14000.msgTs.Key().Cid()
	require.NoError(t, err, "tipset cid")

	noCollectedEvents := []*CollectedEvent{}
	oneCollectedEvent := []*CollectedEvent{
		{
			Entries:     ev1.Entries,
			EmitterAddr: a1,
			EventIdx:    0,
			Reverted:    false,
			Height:      14000,
			TipSetKey:   events14000.msgTs.Key(),
			MsgIdx:      0,
			MsgCid:      em.msg.Cid(),
		},
	}

	workDir, err := os.MkdirTemp("", "lotusevents")
	require.NoError(t, err, "create temporary work directory")

	defer func() {
		_ = os.RemoveAll(workDir)
	}()
	t.Logf("using work dir %q", workDir)

	dbPath := filepath.Join(workDir, "actorevents.db")

	ei, err := NewEventIndex(context.Background(), dbPath, nil)
	require.NoError(t, err, "create event index")
	if err := ei.CollectEvents(context.Background(), events14000, false, addrMap.ResolveAddress); err != nil {
		require.NoError(t, err, "collect events")
	}

	testCases := []struct {
		name   string
		filter *eventFilter
		te     *TipSetEvents
		want   []*CollectedEvent
	}{
		{
			name: "nomatch tipset min height",
			filter: &eventFilter{
				minHeight: 14001,
				maxHeight: -1,
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch tipset max height",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: 13999,
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match tipset min height",
			filter: &eventFilter{
				minHeight: 14000,
				maxHeight: -1,
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match tipset cid",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				tipsetCid: cid14000,
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "nomatch address",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				addresses: []address.Address{a2},
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match address",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				addresses: []address.Address{a1},
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match one entry",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
				}),
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match one entry with alternate values",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("cancel"),
						[]byte("propose"),
						[]byte("approval"),
					},
				}),
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "nomatch one entry by missing value",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("cancel"),
						[]byte("propose"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry by missing key",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"method": {
						[]byte("approval"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match one entry with multiple keys",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"signer": {
						[]byte("addr1"),
					},
				}),
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "nomatch one entry with one mismatching key",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"approver": {
						[]byte("addr1"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry with one mismatching value",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"signer": {
						[]byte("addr2"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry with one unindexed key",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"amount": {
						[]byte("2988181"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
	}

	for _, tc := range testCases {
		tc := tc // appease lint
		t.Run(tc.name, func(t *testing.T) {
			if err := ei.prefillFilter(context.Background(), tc.filter, false); err != nil {
				require.NoError(t, err, "prefill filter events")
			}

			coll := tc.filter.TakeCollectedEvents(context.Background())
			require.ElementsMatch(t, coll, tc.want)
		})
	}
}

func TestEventIndexPrefillFilterExcludeReverted(t *testing.T) {
	rng := pseudo.New(pseudo.NewSource(299792458))
	a1 := randomF4Addr(t, rng)
	a2 := randomF4Addr(t, rng)
	a3 := randomF4Addr(t, rng)

	a1ID := abi.ActorID(1)
	a2ID := abi.ActorID(2)

	addrMap := addressMap{}
	addrMap.add(a1ID, a1)
	addrMap.add(a2ID, a2)

	ev1 := fakeEvent(
		a1ID,
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr1")},
		},
		[]kv{
			{k: "amount", v: []byte("2988181")},
		},
	)
	ev2 := fakeEvent(
		a2ID,
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr2")},
		},
		[]kv{
			{k: "amount", v: []byte("2988182")},
		},
	)

	st := newStore()
	events := []*types.Event{ev1}
	revertedEvents := []*types.Event{ev2}
	em := executedMessage{
		msg: fakeMessage(randomF4Addr(t, rng), randomF4Addr(t, rng)),
		rct: fakeReceipt(t, rng, st, events),
		evs: events,
	}
	revertedEm := executedMessage{
		msg: fakeMessage(randomF4Addr(t, rng), randomF4Addr(t, rng)),
		rct: fakeReceipt(t, rng, st, revertedEvents),
		evs: revertedEvents,
	}

	events14000 := buildTipSetEvents(t, rng, 14000, em)
	revertedEvents14000 := buildTipSetEvents(t, rng, 14000, revertedEm)
	cid14000, err := events14000.msgTs.Key().Cid()
	require.NoError(t, err, "tipset cid")
	reveredCID14000, err := revertedEvents14000.msgTs.Key().Cid()
	require.NoError(t, err, "tipset cid")

	noCollectedEvents := []*CollectedEvent{}
	oneCollectedEvent := []*CollectedEvent{
		{
			Entries:     ev1.Entries,
			EmitterAddr: a1,
			EventIdx:    0,
			Reverted:    false,
			Height:      14000,
			TipSetKey:   events14000.msgTs.Key(),
			MsgIdx:      0,
			MsgCid:      em.msg.Cid(),
		},
	}
	twoCollectedEvent := []*CollectedEvent{
		{
			Entries:     ev1.Entries,
			EmitterAddr: a1,
			EventIdx:    0,
			Reverted:    false,
			Height:      14000,
			TipSetKey:   events14000.msgTs.Key(),
			MsgIdx:      0,
			MsgCid:      em.msg.Cid(),
		},
		{
			Entries:     ev2.Entries,
			EmitterAddr: a2,
			EventIdx:    0,
			Reverted:    true,
			Height:      14000,
			TipSetKey:   revertedEvents14000.msgTs.Key(),
			MsgIdx:      0,
			MsgCid:      revertedEm.msg.Cid(),
		},
	}
	oneCollectedRevertedEvent := []*CollectedEvent{
		{
			Entries:     ev2.Entries,
			EmitterAddr: a2,
			EventIdx:    0,
			Reverted:    true,
			Height:      14000,
			TipSetKey:   revertedEvents14000.msgTs.Key(),
			MsgIdx:      0,
			MsgCid:      revertedEm.msg.Cid(),
		},
	}

	workDir, err := os.MkdirTemp("", "lotusevents")
	require.NoError(t, err, "create temporary work directory")

	defer func() {
		_ = os.RemoveAll(workDir)
	}()
	t.Logf("using work dir %q", workDir)

	dbPath := filepath.Join(workDir, "actorevents.db")

	ei, err := NewEventIndex(context.Background(), dbPath, nil)
	require.NoError(t, err, "create event index")
	if err := ei.CollectEvents(context.Background(), revertedEvents14000, false, addrMap.ResolveAddress); err != nil {
		require.NoError(t, err, "collect reverted events")
	}
	if err := ei.CollectEvents(context.Background(), revertedEvents14000, true, addrMap.ResolveAddress); err != nil {
		require.NoError(t, err, "revert reverted events")
	}
	if err := ei.CollectEvents(context.Background(), events14000, false, addrMap.ResolveAddress); err != nil {
		require.NoError(t, err, "collect events")
	}

	inclusiveTestCases := []struct {
		name   string
		filter *eventFilter
		te     *TipSetEvents
		want   []*CollectedEvent
	}{
		{
			name: "nomatch tipset min height",
			filter: &eventFilter{
				minHeight: 14001,
				maxHeight: -1,
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch tipset max height",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: 13999,
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match tipset min height",
			filter: &eventFilter{
				minHeight: 14000,
				maxHeight: -1,
			},
			te:   events14000,
			want: twoCollectedEvent,
		},
		{
			name: "match tipset cid",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				tipsetCid: cid14000,
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match tipset cid",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				tipsetCid: reveredCID14000,
			},
			te:   revertedEvents14000,
			want: oneCollectedRevertedEvent,
		},
		{
			name: "nomatch address",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				addresses: []address.Address{a3},
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match address 2",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				addresses: []address.Address{a2},
			},
			te:   revertedEvents14000,
			want: oneCollectedRevertedEvent,
		},
		{
			name: "match address 1",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				addresses: []address.Address{a1},
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match one entry",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
				}),
			},
			te:   events14000,
			want: twoCollectedEvent,
		},
		{
			name: "match one entry with alternate values",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("cancel"),
						[]byte("propose"),
						[]byte("approval"),
					},
				}),
			},
			te:   events14000,
			want: twoCollectedEvent,
		},
		{
			name: "nomatch one entry by missing value",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("cancel"),
						[]byte("propose"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry by missing key",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"method": {
						[]byte("approval"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match one entry with multiple keys",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"signer": {
						[]byte("addr1"),
					},
				}),
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match one entry with multiple keys",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"signer": {
						[]byte("addr2"),
					},
				}),
			},
			te:   revertedEvents14000,
			want: oneCollectedRevertedEvent,
		},
		{
			name: "nomatch one entry with one mismatching key",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"approver": {
						[]byte("addr1"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry with one mismatching value",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"signer": {
						[]byte("addr3"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry with one unindexed key",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"amount": {
						[]byte("2988181"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry with one unindexed key",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"amount": {
						[]byte("2988182"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
	}

	exclusiveTestCases := []struct {
		name   string
		filter *eventFilter
		te     *TipSetEvents
		want   []*CollectedEvent
	}{
		{
			name: "nomatch tipset min height",
			filter: &eventFilter{
				minHeight: 14001,
				maxHeight: -1,
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch tipset max height",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: 13999,
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match tipset min height",
			filter: &eventFilter{
				minHeight: 14000,
				maxHeight: -1,
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match tipset cid",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				tipsetCid: cid14000,
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match tipset cid but reverted",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				tipsetCid: reveredCID14000,
			},
			te:   revertedEvents14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch address",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				addresses: []address.Address{a3},
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch address 2 but reverted",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				addresses: []address.Address{a2},
			},
			te:   revertedEvents14000,
			want: noCollectedEvents,
		},
		{
			name: "match address",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				addresses: []address.Address{a1},
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match one entry",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
				}),
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match one entry with alternate values",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("cancel"),
						[]byte("propose"),
						[]byte("approval"),
					},
				}),
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "nomatch one entry by missing value",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("cancel"),
						[]byte("propose"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry by missing key",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"method": {
						[]byte("approval"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match one entry with multiple keys",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"signer": {
						[]byte("addr1"),
					},
				}),
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "nomatch one entry with one mismatching key",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"approver": {
						[]byte("addr1"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry with matching reverted value",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"signer": {
						[]byte("addr2"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry with one mismatching value",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"signer": {
						[]byte("addr3"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry with one unindexed key",
			filter: &eventFilter{
				minHeight: -1,
				maxHeight: -1,
				keysWithCodec: keysToKeysWithCodec(map[string][][]byte{
					"amount": {
						[]byte("2988181"),
					},
				}),
			},
			te:   events14000,
			want: noCollectedEvents,
		},
	}

	for _, tc := range inclusiveTestCases {
		tc := tc // appease lint
		t.Run(tc.name, func(t *testing.T) {
			if err := ei.prefillFilter(context.Background(), tc.filter, false); err != nil {
				require.NoError(t, err, "prefill filter events")
			}

			coll := tc.filter.TakeCollectedEvents(context.Background())
			require.ElementsMatch(t, coll, tc.want, tc.name)
		})
	}

	for _, tc := range exclusiveTestCases {
		tc := tc // appease lint
		t.Run(tc.name, func(t *testing.T) {
			if err := ei.prefillFilter(context.Background(), tc.filter, true); err != nil {
				require.NoError(t, err, "prefill filter events")
			}

			coll := tc.filter.TakeCollectedEvents(context.Background())
			require.ElementsMatch(t, coll, tc.want, tc.name)
		})
	}
}
