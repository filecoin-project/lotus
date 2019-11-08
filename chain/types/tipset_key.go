package types

import (
	"bytes"
	"encoding/binary"
	"strings"

	"github.com/ipfs/go-cid"
)

// A TipSetKey is an immutable collection of CIDs forming a unique key for a tipset.
// The CIDs are assumed to be distinct and in canonical order. Two keys with the same
// CIDs in a different order are not considered equal.
// TipSetKey is a lightweight value type, and may be compared for equality with ==.
type TipSetKey struct {
	// The internal representation is a concatenation of the bytes of the CIDs, each preceded by
	// uint32 specifying the length of the CIDs bytes, and the whole preceded by a uint32
	// giving the number of CIDs.
	// These gymnastics make the a TipSetKey usable as a map key.
	// This representation is slightly larger than strictly necessary: CIDs do carry a prefix
	// including their length, but the cid package doesn't quite expose the functions needed to
	// safely parse a sequence of concatenated CIDs from a stream, and we probably don't want to
	// (re-)implement that here. See https://github.com/ipfs/go-cid/issues/93.
	// The empty key has value "" (no encoded-zero prefix).
	value string
}

// NewTipSetKey builds a new key from a slice of CIDs.
// The CIDs are assumed to be ordered correctly.
func NewTipSetKey(cids ...cid.Cid) TipSetKey {
	encoded, err := encodeKey(cids)
	if err != nil {
		panic("failed to encode CIDs: " + err.Error())
	}
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

func encodeKey(cids []cid.Cid) ([]byte, error) {
	length := uint32(len(cids))
	if length == uint32(0) {
		return []byte{}, nil
	}
	buffer := new(bytes.Buffer)
	err := binary.Write(buffer, binary.LittleEndian, length)
	if err != nil {
		return nil, err
	}
	for _, c := range cids {
		b := c.Bytes()
		l := uint32(len(b))
		err = binary.Write(buffer, binary.LittleEndian, l)
		if err != nil {
			return nil, err
		}
		err = binary.Write(buffer, binary.LittleEndian, c.Bytes())
		if err != nil {
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

func decodeKey(encoded []byte) ([]cid.Cid, error) {
	if len(encoded) == 0 {
		return []cid.Cid{}, nil
	}

	buffer := bytes.NewReader(encoded)
	var length uint32
	err := binary.Read(buffer, binary.LittleEndian, &length)
	if err != nil {
		return nil, err
	}

	var cids []cid.Cid
	for idx := uint32(0); idx < length; idx++ {
		var l uint32
		err = binary.Read(buffer, binary.LittleEndian, &l)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, l)
		err = binary.Read(buffer, binary.LittleEndian, &buf)
		if err != nil {
			return nil, err
		}
		blockCid, err := cid.Cast(buf)
		if err != nil {
			return nil, err
		}
		cids = append(cids, blockCid)
	}
	return cids, nil
}