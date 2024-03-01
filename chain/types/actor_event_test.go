package types

import (
	"encoding/json"
	pseudo "math/rand"
	"testing"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
)

func TestJSONMarshalling(t *testing.T) {
	rng := pseudo.New(pseudo.NewSource(0))
	t.Run("actor event with entries",
		testJsonMarshalling(
			ActorEvent{
				Entries: []EventEntry{
					{
						Key:   "key1",
						Codec: 0x51,
						Value: []byte("value1"),
					},
					{
						Key:   "key2",
						Codec: 0x52,
						Value: []byte("value2"),
					},
				},
				Emitter:   randomF4Addr(t, rng),
				Reverted:  false,
				Height:    1001,
				TipSetKey: NewTipSetKey(randomCid(t, rng)),
				MsgCid:    randomCid(t, rng),
			},
			`{"entries":[{"Flags":0,"Key":"key1","Codec":81,"Value":"dmFsdWUx"},{"Flags":0,"Key":"key2","Codec":82,"Value":"dmFsdWUy"}],"emitter":"f410fagkp3qx2f76maqot74jaiw3tzbxe76k76zrkl3xifk67isrnbn2sll3yua","reverted":false,"height":1001,"tipsetKey":[{"/":"bafkqacx3dag26sfht3qlcdi"}],"msgCid":{"/":"bafkqacrziziykd6uuf4islq"}}`,
		),
	)

	t.Run("actor event filter",
		testJsonMarshalling(
			ActorEventFilter{
				Addresses: []address.Address{
					randomF4Addr(t, pseudo.New(pseudo.NewSource(0))),
					randomF4Addr(t, pseudo.New(pseudo.NewSource(0))),
				},
				Fields: map[string][]ActorEventBlock{
					"key1": {
						{
							Codec: 0x51,
							Value: []byte("value1"),
						},
					},
					"key2": {
						{
							Codec: 0x52,
							Value: []byte("value2"),
						},
					},
				},
				FromHeight: heightOf(0),
				ToHeight:   heightOf(100),
				TipSetKey:  randomTipSetKey(t, rng),
			},
			`{"addresses":["f410fagkp3qx2f76maqot74jaiw3tzbxe76k76zrkl3xifk67isrnbn2sll3yua","f410fagkp3qx2f76maqot74jaiw3tzbxe76k76zrkl3xifk67isrnbn2sll3yua"],"fields":{"key1":[{"codec":81,"value":"dmFsdWUx"}],"key2":[{"codec":82,"value":"dmFsdWUy"}]},"fromHeight":0,"toHeight":100,"tipsetKey":[{"/":"bafkqacxcqxwocuiukv4aq5i"}]}`,
		),
	)
	t.Run("actor event block",
		testJsonMarshalling(
			ActorEventBlock{
				Codec: 1,
				Value: []byte("test"),
			},
			`{"codec":1,"value":"dGVzdA=="}`,
		),
	)
}

func testJsonMarshalling[V ActorEvent | ActorEventBlock | ActorEventFilter](subject V, expect string) func(t *testing.T) {
	return func(t *testing.T) {
		gotMarshalled, err := json.Marshal(subject)
		require.NoError(t, err)
		require.JSONEqf(t, expect, string(gotMarshalled), "serialization mismatch")
		var gotUnmarshalled V
		require.NoError(t, json.Unmarshal([]byte(expect), &gotUnmarshalled))
		require.Equal(t, subject, gotUnmarshalled)
	}
}

func heightOf(h int64) *abi.ChainEpoch {
	hp := abi.ChainEpoch(h)
	return &hp
}

func randomTipSetKey(tb testing.TB, rng *pseudo.Rand) *TipSetKey {
	tb.Helper()
	tk := NewTipSetKey(randomCid(tb, rng))
	return &tk
}

func randomF4Addr(tb testing.TB, rng *pseudo.Rand) address.Address {
	tb.Helper()
	addr, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, randomBytes(32, rng))
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
