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

	ei, err := NewEventIndex(dbPath)
	require.NoError(t, err, "create event index")
	if err := ei.CollectEvents(context.Background(), events14000, false, addrMap.ResolveAddress); err != nil {
		require.NoError(t, err, "collect events")
	}

	testCases := []struct {
		name   string
		filter *EventFilter
		te     *TipSetEvents
		want   []*CollectedEvent
	}{
		{
			name: "nomatch tipset min height",
			filter: &EventFilter{
				minHeight: 14001,
				maxHeight: -1,
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch tipset max height",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: 13999,
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match tipset min height",
			filter: &EventFilter{
				minHeight: 14000,
				maxHeight: -1,
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match tipset cid",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				tipsetCid: cid14000,
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "nomatch address",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				addresses: []address.Address{a2},
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match address",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				addresses: []address.Address{a1},
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match one entry",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				keys: map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
				},
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "match one entry with alternate values",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				keys: map[string][][]byte{
					"type": {
						[]byte("cancel"),
						[]byte("propose"),
						[]byte("approval"),
					},
				},
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "nomatch one entry by missing value",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				keys: map[string][][]byte{
					"type": {
						[]byte("cancel"),
						[]byte("propose"),
					},
				},
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry by missing key",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				keys: map[string][][]byte{
					"method": {
						[]byte("approval"),
					},
				},
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "match one entry with multiple keys",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				keys: map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"signer": {
						[]byte("addr1"),
					},
				},
			},
			te:   events14000,
			want: oneCollectedEvent,
		},
		{
			name: "nomatch one entry with one mismatching key",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				keys: map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"approver": {
						[]byte("addr1"),
					},
				},
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry with one mismatching value",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				keys: map[string][][]byte{
					"type": {
						[]byte("approval"),
					},
					"signer": {
						[]byte("addr2"),
					},
				},
			},
			te:   events14000,
			want: noCollectedEvents,
		},
		{
			name: "nomatch one entry with one unindexed key",
			filter: &EventFilter{
				minHeight: -1,
				maxHeight: -1,
				keys: map[string][][]byte{
					"amount": {
						[]byte("2988181"),
					},
				},
			},
			te:   events14000,
			want: noCollectedEvents,
		},
	}

	for _, tc := range testCases {
		tc := tc // appease lint
		t.Run(tc.name, func(t *testing.T) {
			if err := ei.PrefillFilter(context.Background(), tc.filter); err != nil {
				require.NoError(t, err, "prefill filter events")
			}

			coll := tc.filter.TakeCollectedEvents(context.Background())
			require.ElementsMatch(t, coll, tc.want)
		})
	}
}
