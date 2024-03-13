// stm: #unit
package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagjson"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cborrpc "github.com/filecoin-project/go-cbor-util"
)

func TestTipSetKey(t *testing.T) {
	//stm: @TYPES_TIPSETKEY_FROM_BYTES_001, @TYPES_TIPSETKEY_NEW_001
	cb := cid.V1Builder{Codec: cid.DagCBOR, MhType: multihash.BLAKE2B_MIN + 31}
	c1, _ := cb.Sum([]byte("a"))
	c2, _ := cb.Sum([]byte("b"))
	c3, _ := cb.Sum([]byte("c"))
	fmt.Println(len(c1.Bytes()))

	t.Run("zero value", func(t *testing.T) {
		assert.Equal(t, EmptyTSK, NewTipSetKey())
	})

	t.Run("CID extraction", func(t *testing.T) {
		assert.Equal(t, []cid.Cid{}, NewTipSetKey().Cids())
		assert.Equal(t, []cid.Cid{c1}, NewTipSetKey(c1).Cids())
		assert.Equal(t, []cid.Cid{c1, c2, c3}, NewTipSetKey(c1, c2, c3).Cids())

		// The key doesn't check for duplicates.
		assert.Equal(t, []cid.Cid{c1, c1}, NewTipSetKey(c1, c1).Cids())
	})

	t.Run("equality", func(t *testing.T) {
		assert.Equal(t, NewTipSetKey(), NewTipSetKey())
		assert.Equal(t, NewTipSetKey(c1), NewTipSetKey(c1))
		assert.Equal(t, NewTipSetKey(c1, c2, c3), NewTipSetKey(c1, c2, c3))

		assert.NotEqual(t, NewTipSetKey(), NewTipSetKey(c1))
		assert.NotEqual(t, NewTipSetKey(c2), NewTipSetKey(c1))
		// The key doesn't normalize order.
		assert.NotEqual(t, NewTipSetKey(c1, c2), NewTipSetKey(c2, c1))
	})

	t.Run("encoding", func(t *testing.T) {
		keys := []TipSetKey{
			NewTipSetKey(),
			NewTipSetKey(c1),
			NewTipSetKey(c1, c2, c3),
		}

		for _, tk := range keys {
			roundTrip, err := TipSetKeyFromBytes(tk.Bytes())
			require.NoError(t, err)
			assert.Equal(t, tk, roundTrip)
		}

		_, err := TipSetKeyFromBytes(NewTipSetKey(c1).Bytes()[1:])
		assert.Error(t, err)
	})

	t.Run("JSON", func(t *testing.T) {
		k0 := NewTipSetKey()
		verifyJSON(t, "[]", k0)
		k3 := NewTipSetKey(c1, c2, c3)
		verifyJSON(t, `[`+
			`{"/":"bafy2bzacecesrkxghscnq7vatble2hqdvwat6ed23vdu4vvo3uuggsoaya7ki"},`+
			`{"/":"bafy2bzacebxfyh2fzoxrt6kcgc5dkaodpcstgwxxdizrww225vrhsizsfcg4g"},`+
			`{"/":"bafy2bzacedwviarjtjraqakob5pslltmuo5n3xev3nt5zylezofkbbv5jclyu"}`+
			`]`, k3)
	})

	t.Run("CBOR", func(t *testing.T) {
		k3 := NewTipSetKey(c1, c2, c3)
		b, err := cborrpc.Dump(k3)
		require.NoError(t, err)
		fmt.Println(hex.EncodeToString(b))
	})
}

func verifyJSON(t *testing.T, expected string, k TipSetKey) {
	bytes, err := json.Marshal(k)
	require.NoError(t, err)
	assert.Equal(t, expected, string(bytes))

	var rehydrated TipSetKey
	err = json.Unmarshal(bytes, &rehydrated)
	require.NoError(t, err)
	assert.Equal(t, k, rehydrated)
}

// Test that our go-ipld-prime bindnode option works with TipSetKey as an Any in the schema but
// properly typed in the Go type. In this form we expect it to match the JSON style encoding, but
// we could use an alternate encoder (dag-cbor) and get the same form: a list of links. This is
// distinct from the natural CBOR form of TipSetKey which is just a byte string, which we don't
// (yet) have a bindnode option for (but could).

var ipldSchema string = `
type TSKHolder struct {
	K1 String
	TSK Any
	K2 String
}
`

type testTSKHolder struct {
	K1  string
	TSK TipSetKey
	K2  string
}

func TestBindnodeTipSetKey(t *testing.T) {
	req := require.New(t)

	cb := cid.V1Builder{Codec: cid.DagCBOR, MhType: multihash.BLAKE2B_MIN + 31}
	c1, _ := cb.Sum([]byte("a"))
	c2, _ := cb.Sum([]byte("b"))
	c3, _ := cb.Sum([]byte("c"))
	tsk := NewTipSetKey(c1, c2, c3)

	typeSystem, err := ipld.LoadSchemaBytes([]byte(ipldSchema))
	req.NoError(err, "should load a properly encoded schema")
	schemaType := typeSystem.TypeByName("TSKHolder")
	req.NotNil(schemaType, "should have the expected type")
	proto := bindnode.Prototype((*testTSKHolder)(nil), schemaType, TipSetKeyAsLinksListBindnodeOption)
	nd := bindnode.Wrap(&testTSKHolder{
		K1:  "before",
		TSK: tsk,
		K2:  "after",
	}, schemaType, TipSetKeyAsLinksListBindnodeOption)
	jsonBytes, err := ipld.Encode(nd, dagjson.Encode)
	req.NoError(err, "should encode to JSON with ipld-prime")

	// plain json unmarshal, make sure we're compatible
	var holder testTSKHolder
	err = json.Unmarshal(jsonBytes, &holder)
	req.NoError(err, "should decode with encoding/json")
	req.Equal("before", holder.K1)
	req.Equal("after", holder.K2)
	req.Equal(tsk, holder.TSK)

	// decode with ipld-prime
	decoded, err := ipld.DecodeUsingPrototype(jsonBytes, dagjson.Decode, proto)
	req.NoError(err, "should decode with ipld-prime")
	tskPtr := bindnode.Unwrap(decoded)
	req.NotNil(tskPtr, "should have a non-nil value")
	holder2, ok := tskPtr.(*testTSKHolder)
	req.True(ok, "should unwrap to the correct go type")
	req.Equal("before", holder2.K1)
	req.Equal("after", holder2.K2)
	req.Equal(tsk, holder2.TSK)
}
