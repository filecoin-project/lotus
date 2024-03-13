package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	typegen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/abi"
)

var EmptyTSK = TipSetKey{}

// The length of a block header CID in bytes.
var blockHeaderCIDLen int

func init() {
	// hash a large string of zeros so we don't estimate based on inlined CIDs.
	var buf [256]byte
	c, err := abi.CidBuilder.Sum(buf[:])
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
		return EmptyTSK, err
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

func (k TipSetKey) Cid() (cid.Cid, error) {
	blk, err := k.ToStorageBlock()
	if err != nil {
		return cid.Cid{}, err
	}
	return blk.Cid(), nil
}

func (k TipSetKey) ToStorageBlock() (block.Block, error) {
	buf := new(bytes.Buffer)
	if err := k.MarshalCBOR(buf); err != nil {
		log.Errorf("failed to marshal ts key as CBOR: %s", k)
	}

	cid, err := abi.CidBuilder.Sum(buf.Bytes())
	if err != nil {
		return nil, err
	}

	return block.NewBlockWithCid(buf.Bytes(), cid)
}

func (k TipSetKey) MarshalCBOR(writer io.Writer) error {
	if err := typegen.WriteMajorTypeHeader(writer, typegen.MajByteString, uint64(len(k.Bytes()))); err != nil {
		return err
	}

	_, err := writer.Write(k.Bytes())
	return err
}

func (k *TipSetKey) UnmarshalCBOR(reader io.Reader) error {
	cr := typegen.NewCborReader(reader)

	maj, extra, err := cr.ReadHeader()
	if err != nil {
		return err
	}
	defer func() {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
	}()

	if extra > typegen.ByteArrayMaxLen {
		return fmt.Errorf("t.Binary: byte array too large (%d)", extra)
	}
	if maj != typegen.MajByteString {
		return fmt.Errorf("expected byte array")
	}

	b := make([]uint8, extra)

	if _, err := io.ReadFull(cr, b); err != nil {
		return err
	}

	*k, err = TipSetKeyFromBytes(b)
	return err
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

var _ typegen.CBORMarshaler = &TipSetKey{}
var _ typegen.CBORUnmarshaler = &TipSetKey{}

// TipSetKeyAsLinksListBindnodeOption Used in conjunction
// github.com/ipld/go-ipld-prime/node/bindnode when you want to refer to a TipSetKey and have it
// encoded as a list of links, as in the JSON representation above rather than as a byte string as
// in the CBOR representation above.
//
// This option lets you represent a TipSetKey field as an Any in a schema, and it will map directly
// on to a TipSetKey in the corresponding concrete Go types.
var TipSetKeyAsLinksListBindnodeOption = bindnode.TypedAnyConverter(&TipSetKey{}, tipsetKeyAsLinksListFromAny, tipsetKeyAsLinksListToAny)

func tipsetKeyAsLinksListFromAny(n datamodel.Node) (interface{}, error) {
	if n.Kind() != datamodel.Kind_List {
		return nil, errors.New("expected list representation")
	}
	cids := make([]cid.Cid, 0)
	itr := n.ListIterator()
	for !itr.Done() {
		_, v, err := itr.Next()
		if err != nil {
			return nil, err
		}
		if v.Kind() != datamodel.Kind_Link {
			return nil, errors.New("expected link")
		}
		l, err := v.AsLink()
		if err != nil {
			return nil, err
		}
		if cl, ok := l.(cidlink.Link); ok {
			cids = append(cids, cl.Cid)
		} else {
			return nil, errors.New("expected CID link")
		}
	}
	return &TipSetKey{value: string(encodeKey(cids))}, nil

}

func tipsetKeyAsLinksListToAny(iface interface{}) (datamodel.Node, error) {
	tsk, ok := iface.(*TipSetKey)
	if !ok {
		return nil, fmt.Errorf("expected *Address value")
	}
	cids := tsk.Cids()
	nb := basicnode.Prototype.List.NewBuilder()
	la, err := nb.BeginList(int64(len(cids)))
	if err != nil {
		return nil, err
	}
	for _, c := range cids {
		if err := la.AssembleValue().AssignLink(cidlink.Link{Cid: c}); err != nil {
			return nil, err
		}
	}
	if err := la.Finish(); err != nil {
		return nil, err
	}
	return nb.Build(), nil
}
