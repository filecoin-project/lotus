package filter

import (
	"context"
	pseudo "math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"
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

	subCh, unSubscribe := ei.SubscribeUpdates()
	defer unSubscribe()

	out := make(chan EventIndexUpdated, 1)
	go func() {
		tu := <-subCh
		out <- tu
	}()

	if err := ei.CollectEvents(context.Background(), events14000, false, addrMap.ResolveAddress); err != nil {
		require.NoError(t, err, "collect events")
	}

	mh, err := ei.GetMaxHeightInIndex(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(14000), mh)

	b, err := ei.IsHeightPast(context.Background(), 14000)
	require.NoError(t, err)
	require.True(t, b)

	b, err = ei.IsHeightPast(context.Background(), 14001)
	require.NoError(t, err)
	require.False(t, b)

	b, err = ei.IsHeightPast(context.Background(), 13000)
	require.NoError(t, err)
	require.True(t, b)

	tsKey := events14000.msgTs.Key()
	tsKeyCid, err := tsKey.Cid()
	require.NoError(t, err, "tipset key cid")

	seen, err := ei.IsTipsetProcessed(context.Background(), tsKeyCid.Bytes())
	require.NoError(t, err)
	require.True(t, seen, "tipset key should be seen")

	seen, err = ei.IsTipsetProcessed(context.Background(), []byte{1})
	require.NoError(t, err)
	require.False(t, seen, "tipset key should not be seen")

	_ = <-out

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

	tCh := make(chan EventIndexUpdated, 3)
	subCh, unSubscribe := ei.SubscribeUpdates()
	defer unSubscribe()
	go func() {
		cnt := 0
		for tu := range subCh {
			tCh <- tu
			cnt++
			if cnt == 3 {
				close(tCh)
				return
			}
		}
	}()

	if err := ei.CollectEvents(context.Background(), revertedEvents14000, false, addrMap.ResolveAddress); err != nil {
		require.NoError(t, err, "collect reverted events")
	}
	if err := ei.CollectEvents(context.Background(), revertedEvents14000, true, addrMap.ResolveAddress); err != nil {
		require.NoError(t, err, "revert reverted events")
	}
	if err := ei.CollectEvents(context.Background(), events14000, false, addrMap.ResolveAddress); err != nil {
		require.NoError(t, err, "collect events")
	}

	_ = <-tCh
	_ = <-tCh
	_ = <-tCh

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

// TestQueryPlan is to ensure that future modifications to the db schema, or future upgrades to
// sqlite, do not change the query plan of the prepared statements used by the event index such that
// queries hit undesirable indexes which are likely to slow down the query.
// Changes that break this test need to be sure that the query plan is still efficient for the
// expected query patterns.
func TestQueryPlan(t *testing.T) {
	ei, err := NewEventIndex(context.Background(), filepath.Join(t.TempDir(), "actorevents.db"), nil)
	require.NoError(t, err, "create event index")

	verifyQueryPlan := func(stmt string) {
		rows, err := ei.db.Query("EXPLAIN QUERY PLAN " + strings.Replace(stmt, "?", "1", -1))
		require.NoError(t, err, "explain query plan for query: "+stmt)
		defer func() {
			require.NoError(t, rows.Close())
		}()
		// First response to EXPLAIN QUERY PLAN should show us the use of an index that we want to
		// encounter first to narrow down the search space - either a height or tipset_key_cid index
		// - sqlite_autoindex_events_seen_1 is for the UNIQUE constraint on events_seen
		// - events_seen_height and events_seen_tipset_key_cid are explicit indexes on events_seen
		// - event_height and event_tipset_key_cid are explicit indexes on event
		rows.Next()
		var id, parent, notused, detail string
		require.NoError(t, rows.Scan(&id, &parent, &notused, &detail), "scan explain query plan for query: "+stmt)
		detail = strings.TrimSpace(detail)
		var expectedIndexes = []string{
			"sqlite_autoindex_events_seen_1",
			"events_seen_height",
			"events_seen_tipset_key_cid",
			"event_height",
			"event_tipset_key_cid",
		}
		indexUsed := false
		for _, index := range expectedIndexes {
			if strings.Contains(detail, " INDEX "+index) {
				indexUsed = true
				break
			}
		}
		require.True(t, indexUsed, "index used for query: "+stmt+" detail: "+detail)

		stmt = regexp.MustCompile(`(?m)^\s+`).ReplaceAllString(stmt, " ") // remove all leading whitespace from the statement
		stmt = strings.Replace(stmt, "\n", "", -1)                        // remove all newlines from the statement
		t.Logf("[%s] has plan start: %s", stmt, detail)
	}

	// Test the hard-coded select and update queries
	stmtMap := preparedStatementMapping(&preparedStatements{})
	for _, stmt := range stmtMap {
		if strings.HasPrefix(strings.TrimSpace(strings.ToLower(stmt)), "insert") {
			continue
		}
		verifyQueryPlan(stmt)
	}

	// Test the dynamic prefillFilter queries
	prefillCases := []*eventFilter{
		{},
		{minHeight: 14000, maxHeight: 14000},
		{minHeight: 14000, maxHeight: 15000},
		{tipsetCid: cid.MustParse("bafkqaaa")},
		{minHeight: 14000, maxHeight: 14000, addresses: []address.Address{address.TestAddress}},
		{minHeight: 14000, maxHeight: 15000, addresses: []address.Address{address.TestAddress}},
		{tipsetCid: cid.MustParse("bafkqaaa"), addresses: []address.Address{address.TestAddress}},
		{minHeight: 14000, maxHeight: 14000, addresses: []address.Address{address.TestAddress, address.TestAddress}},
		{minHeight: 14000, maxHeight: 15000, addresses: []address.Address{address.TestAddress, address.TestAddress}},
		{tipsetCid: cid.MustParse("bafkqaaa"), addresses: []address.Address{address.TestAddress, address.TestAddress}},
		{minHeight: 14000, maxHeight: 14000, keysWithCodec: keysToKeysWithCodec(map[string][][]byte{"type": {[]byte("approval")}})},
		{minHeight: 14000, maxHeight: 15000, keysWithCodec: keysToKeysWithCodec(map[string][][]byte{"type": {[]byte("approval")}})},
		{tipsetCid: cid.MustParse("bafkqaaa"), keysWithCodec: keysToKeysWithCodec(map[string][][]byte{"type": {[]byte("approval")}})},
		{minHeight: 14000, maxHeight: 14000, keysWithCodec: keysToKeysWithCodec(map[string][][]byte{"type": {[]byte("approval")}, "signer": {[]byte("addr1")}})},
		{minHeight: 14000, maxHeight: 15000, keysWithCodec: keysToKeysWithCodec(map[string][][]byte{"type": {[]byte("approval")}, "signer": {[]byte("addr1")}})},
		{tipsetCid: cid.MustParse("bafkqaaa"), keysWithCodec: keysToKeysWithCodec(map[string][][]byte{"type": {[]byte("approval")}, "signer": {[]byte("addr1")}})},
		{minHeight: 14000, maxHeight: 14000, addresses: []address.Address{address.TestAddress, address.TestAddress}, keysWithCodec: keysToKeysWithCodec(map[string][][]byte{"type": {[]byte("approval")}, "signer": {[]byte("addr1")}})},
		{minHeight: 14000, maxHeight: 15000, addresses: []address.Address{address.TestAddress, address.TestAddress}, keysWithCodec: keysToKeysWithCodec(map[string][][]byte{"type": {[]byte("approval")}, "signer": {[]byte("addr1")}})},
		{tipsetCid: cid.MustParse("bafkqaaa"), addresses: []address.Address{address.TestAddress, address.TestAddress}, keysWithCodec: keysToKeysWithCodec(map[string][][]byte{"type": {[]byte("approval")}, "signer": {[]byte("addr1")}})},
	}
	for _, filter := range prefillCases {
		_, query := makePrefillFilterQuery(filter, true)
		verifyQueryPlan(query)
		_, query = makePrefillFilterQuery(filter, false)
		verifyQueryPlan(query)
	}
}
