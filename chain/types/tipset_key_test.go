package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cborrpc "github.com/filecoin-project/go-cbor-util"
)

func TestTipSetKey(t *testing.T) {
	cb := cid.V1Builder{Codec: cid.DagCBOR, MhType: multihash.BLAKE2B_MIN + 31}
	// distinct, but arbitrary CIDs, pretending dag-cbor encoding but they are just a multihash over bytes
	c1, _ := cb.Sum([]byte("a"))
	c2, _ := cb.Sum([]byte("b"))
	c3, _ := cb.Sum([]byte("c"))

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

	t.Run("derived CID", func(t *testing.T) {
		assert.Equal(t, "bafy2bzacecesrkxghscnq7vatble2hqdvwat6ed23vdu4vvo3uuggsoaya7ki", c1.String()) // sanity check
		actualCid, err := NewTipSetKey().Cid()
		require.NoError(t, err)
		assert.Equal(t, "bafy2bzacea456askyutsf7uk4ta2q5aojrlcji4mhaqokbfalgvoq4ueeh4l2", actualCid.String(), "empty TSK")
		actualCid, err = NewTipSetKey(c1).Cid()
		require.NoError(t, err)
		assert.Equal(t, "bafy2bzacealem6larzxhf7aggj3cozcefqez3jlksx2tuxehwdil27otcmy4q", actualCid.String())
		actualCid, err = NewTipSetKey(c1, c2, c3).Cid()
		require.NoError(t, err)
		assert.Equal(t, "bafy2bzacecbnwngwfvxuciumcfudiaoqozisp3hus5im5lg4urrwlxbueissu", actualCid.String())

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
