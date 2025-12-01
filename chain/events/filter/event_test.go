package filter

import (
	"context"
	pseudo "math/rand"
	"testing"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types"
)

func keysToKeysWithCodec(keys map[string][][]byte) map[string][]types.ActorEventBlock {
	keysWithCodec := make(map[string][]types.ActorEventBlock)
	for k, v := range keys {
		for _, vv := range v {
			keysWithCodec[k] = append(keysWithCodec[k], types.ActorEventBlock{
				Codec: cid.Raw,
				Value: vv,
			})
		}
	}
	return keysWithCodec
}

func TestEventFilterCollectEvents(t *testing.T) {
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

	noCollectedEvents := []*index.CollectedEvent{}
	oneCollectedEvent := []*index.CollectedEvent{
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

	testCases := []struct {
		name   string
		filter *eventFilter
		te     *TipSetEvents
		want   []*index.CollectedEvent
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
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.filter.CollectEvents(context.Background(), tc.te, false, addrMap.ResolveAddress); err != nil {
				require.NoError(t, err, "collect events")
			}

			coll := tc.filter.TakeCollectedEvents(context.Background())
			require.ElementsMatch(t, coll, tc.want)
		})
	}
}

type kv struct {
	k string
	v []byte
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

func randomF4Addr(tb testing.TB, rng *pseudo.Rand) address.Address {
	tb.Helper()
	addr, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, randomBytes(32, rng))
	require.NoError(tb, err)

	return addr
}

func randomIDAddr(tb testing.TB, rng *pseudo.Rand) address.Address {
	tb.Helper()
	addr, err := address.NewIDAddress(uint64(rng.Int63()))
	require.NoError(tb, err)
	return addr
}

func randomCid(tb testing.TB, rng *pseudo.Rand) cid.Cid {
	tb.Helper()
	cb := cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}
	c, err := cb.Sum(randomBytes(10, rng))
	require.NoError(tb, err)
	return c
}

func randomBytes(n int, rng *pseudo.Rand) []byte {
	buf := make([]byte, n)
	rng.Read(buf)
	return buf
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

func fakeReceipt(tb testing.TB, rng *pseudo.Rand, st adt.Store, events []*types.Event) *types.MessageReceipt {
	arr := blockadt.MakeEmptyArray(st)
	for _, ev := range events {
		err := arr.AppendContinuous(ev)
		require.NoError(tb, err, "append event")
	}
	eventsRoot, err := arr.Root()
	require.NoError(tb, err, "flush events amt")

	rec := types.NewMessageReceiptV1(exitcode.Ok, randomBytes(32, rng), rng.Int63(), &eventsRoot)
	return &rec
}

func fakeTipSet(tb testing.TB, rng *pseudo.Rand, h abi.ChainEpoch, parents []cid.Cid) *types.TipSet {
	tb.Helper()
	ts, err := types.NewTipSet([]*types.BlockHeader{
		{
			Height: h,
			Miner:  randomIDAddr(tb, rng),

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte(h % 2)}},

			ParentStateRoot:       randomCid(tb, rng),
			Messages:              randomCid(tb, rng),
			ParentMessageReceipts: randomCid(tb, rng),

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
		{
			Height: h,
			Miner:  randomIDAddr(tb, rng),

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte((h + 1) % 2)}},

			ParentStateRoot:       randomCid(tb, rng),
			Messages:              randomCid(tb, rng),
			ParentMessageReceipts: randomCid(tb, rng),

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
	})

	require.NoError(tb, err)

	return ts
}

func newStore() adt.Store {
	ctx := context.Background()
	bs := blockstore.NewMemorySync()
	store := cbor.NewCborStore(bs)
	return adt.WrapStore(ctx, store)
}

func buildTipSetEvents(tb testing.TB, rng *pseudo.Rand, h abi.ChainEpoch, em executedMessage) *TipSetEvents {
	tb.Helper()

	msgTs := fakeTipSet(tb, rng, h, []cid.Cid{})
	rctTs := fakeTipSet(tb, rng, h+1, msgTs.Cids())

	return &TipSetEvents{
		msgTs: msgTs,
		rctTs: rctTs,
		load: func(ctx context.Context, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
			return []executedMessage{em}, nil
		},
	}
}

type addressMap map[abi.ActorID]address.Address

func (a addressMap) add(actorID abi.ActorID, addr address.Address) {
	a[actorID] = addr
}

func (a addressMap) ResolveAddress(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) address.Address {
	ra, ok := a[emitter]
	if ok {
		return ra
	}
	return address.Undef
}
