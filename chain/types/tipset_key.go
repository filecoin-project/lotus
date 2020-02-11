package types

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

var EmptyTSK = TipSetKey{}

// The length of a block header CID in bytes.
var blockHeaderCIDLen int

func init() {
	c, err := cid.V1Builder{Codec: cid.DagCBOR, MhType: multihash.BLAKE2B_MIN + 31}.Sum([]byte{})
	if err != nil {
		panic(err)
	}
	blockHeaderCIDLen = len(c.Bytes())
}

// A TipSetKey is an immutable collection of CIDs forming a unique key for a tipset.
// The CIDs are assumed to be distinct and in canonical order. Two keys with the same
// CIDs in a different order are not considered equal.
// TipSetKey is a lightweight value type, and may be compared for equality with ==.
type TipSetKey struct {
	// The internal representation is a concatenation of the bytes of the CIDs, which are
	// self-describing, wrapped as a string.
	// These gymnastics make the a TipSetKey usable as a map key.
	// The empty key has value "".
	value string
}

// NewTipSetKey builds a new key from a slice of CIDs.
// The CIDs are assumed to be ordered correctly.
func NewTipSetKey(cids ...cid.Cid) TipSetKey {
	encoded := encodeKey(cids)
	return TipSetKey{string(encoded)}
}

// TipSetKeyFromBytes wraps an encoded key, validating correct decoding.
func TipSetKeyFromBytes(encoded []byte) (TipSetKey, error) {
	_, err := decodeKey(encoded)
	if err != nil {
		return TipSetKey{}, err
	}
	return TipSetKey{string(encoded)}, nil
}

// Cids returns a slice of the CIDs comprising this key.
func (k TipSetKey) Cids() []cid.Cid {
	cids, err := decodeKey([]byte(k.value))
	if err != nil {
		panic("invalid tipset key: " + err.Error())
	}
	return cids
}

// String() returns a human-readable representation of the key.
func (k TipSetKey) String() string {
	b := strings.Builder{}
	b.WriteString("{")
	cids := k.Cids()
	for i, c := range cids {
		b.WriteString(c.String())
		if i < len(cids)-1 {
			b.WriteString(",")
		}
	}
	b.WriteString("}")
	return b.String()
}

// Bytes() returns a binary representation of the key.
func (k TipSetKey) Bytes() []byte {
	return []byte(k.value)
}

func (k TipSetKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.Cids())
}

func (k *TipSetKey) UnmarshalJSON(b []byte) error {
	var cids []cid.Cid
	if err := json.Unmarshal(b, &cids); err != nil {
		return err
	}
	k.value = string(encodeKey(cids))
	return nil
}

func (k TipSetKey) IsEmpty() bool {
	return len(k.value) == 0
}

func encodeKey(cids []cid.Cid) []byte {
	buffer := new(bytes.Buffer)
	for _, c := range cids {
		// bytes.Buffer.Write() err is documented to be always nil.
		_, _ = buffer.Write(c.Bytes())
	}
	return buffer.Bytes()
}

func decodeKey(encoded []byte) ([]cid.Cid, error) {
	// To avoid reallocation of the underlying array, estimate the number of CIDs to be extracted
	// by dividing the encoded length by the expected CID length.
	estimatedCount := len(encoded) / blockHeaderCIDLen
	cids := make([]cid.Cid, 0, estimatedCount)
	nextIdx := 0
	for nextIdx < len(encoded) {
		nr, c, err := cid.CidFromBytes(encoded[nextIdx:])
		if err != nil {
			return nil, err
		}
		cids = append(cids, c)
		nextIdx += nr
	}
	return cids, nil
}
