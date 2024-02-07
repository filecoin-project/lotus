package types

import (
	"encoding/json"
	pseudo "math/rand"
	"testing"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
)

func TestActorEventJson(t *testing.T) {
	// generate a mock Actor event for me
	rng := pseudo.New(pseudo.NewSource(0))
	in := ActorEvent{
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
		EmitterAddr: randomF4Addr(t, rng),
		Reverted:    false,
		Height:      1001,
		TipSetKey:   randomCid(t, rng),
		MsgCid:      randomCid(t, rng),
	}

	bz, err := json.Marshal(in)
	require.NoError(t, err)
	require.NotEmpty(t, bz)

	var out ActorEvent
	err = json.Unmarshal(bz, &out)
	require.NoError(t, err)
	require.Equal(t, in, out)

	s := `
{"entries":[{"Flags":0,"Key":"key1","Codec":81,"Value":"dmFsdWUx"},{"Flags":0,"Key":"key2","Codec":82,"Value":"dmFsdWUy"}],"emitter":"f410fagkp3qx2f76maqot74jaiw3tzbxe76k76zrkl3xifk67isrnbn2sll3yua","reverted":false,"height":1001,"tipset_cid":{"/":"bafkqacx3dag26sfht3qlcdi"},"msg_cid":{"/":"bafkqacrziziykd6uuf4islq"}}
`
	var out2 ActorEvent
	err = json.Unmarshal([]byte(s), &out2)
	require.NoError(t, err)
	require.Equal(t, out, out2)
}

func TestActorEventBlockJson(t *testing.T) {
	in := ActorEventBlock{
		Codec: 1,
		Value: []byte("test"),
	}

	bz, err := json.Marshal(in)
	require.NoError(t, err)
	require.NotEmpty(t, bz)

	var out ActorEventBlock
	err = json.Unmarshal(bz, &out)
	require.NoError(t, err)
	require.Equal(t, in, out)

	var out2 ActorEventBlock
	s := "{\"codec\":1,\"value\":\"dGVzdA==\"}"
	err = json.Unmarshal([]byte(s), &out2)
	require.NoError(t, err)
	require.Equal(t, in, out2)
}

func TestSubActorEventFilterJson(t *testing.T) {
	c := randomCid(t, pseudo.New(pseudo.NewSource(0)))
	from := "earliest"
	to := "latest"
	f := ActorEventFilter{
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
		FromEpoch: from,
		ToEpoch:   to,
		TipSetCid: &c,
	}

	bz, err := json.Marshal(f)
	require.NoError(t, err)
	require.NotEmpty(t, bz)

	s := `{"addresses":["f410fagkp3qx2f76maqot74jaiw3tzbxe76k76zrkl3xifk67isrnbn2sll3yua","f410fagkp3qx2f76maqot74jaiw3tzbxe76k76zrkl3xifk67isrnbn2sll3yua"],"fields":{"key1":[{"codec":81,"value":"dmFsdWUx"}],"key2":[{"codec":82,"value":"dmFsdWUy"}]},"fromEpoch":"earliest","toEpoch":"latest","tipsetCid":{"/":"bafkqacqbst64f6rp7taeduy"}}`
	var out ActorEventFilter
	err = json.Unmarshal([]byte(s), &out)
	require.NoError(t, err)
	require.Equal(t, f, out)
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
