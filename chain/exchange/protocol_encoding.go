package exchange

import (
	"fmt"
	"io"

	cbg "github.com/whyrusleeping/cbor-gen"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build/buildconstants"
	types "github.com/filecoin-project/lotus/chain/types"
)

// CompactedMessagesCBOR is used for encoding/decoding compacted messages. This is a custom type as we need custom limits.
// - Max messages is 150,000 as that's 15 times the max block size (in messages). It needs to be
// large enough to cover a full tipset full of full blocks.
type CompactedMessagesCBOR struct {
	Bls         []*types.Message `cborgen:"maxlen=150000"`
	BlsIncludes []messageIndices

	Secpk         []*types.SignedMessage `cborgen:"maxlen=150000"`
	SecpkIncludes []messageIndices
}

// UnmarshalCBOR unmarshals into the "decoding" struct, then copies into the actual struct.
func (t *CompactedMessages) UnmarshalCBOR(r io.Reader) (err error) {
	var c CompactedMessagesCBOR
	if err := c.UnmarshalCBOR(r); err != nil {
		return err
	}
	t.Bls = c.Bls
	t.BlsIncludes = make([][]uint64, len(c.BlsIncludes))
	for i, v := range c.BlsIncludes {
		t.BlsIncludes[i] = v.v
	}
	t.Secpk = c.Secpk
	t.SecpkIncludes = make([][]uint64, len(c.SecpkIncludes))
	for i, v := range c.SecpkIncludes {
		t.SecpkIncludes[i] = v.v
	}
	return nil
}

// MarshalCBOR copies into the encoding struct, then marshals.
func (t *CompactedMessages) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}

	var c CompactedMessagesCBOR
	c.Bls = t.Bls
	c.BlsIncludes = make([]messageIndices, len(t.BlsIncludes))
	for i, v := range t.BlsIncludes {
		c.BlsIncludes[i].v = v
	}
	c.Secpk = t.Secpk
	c.SecpkIncludes = make([]messageIndices, len(t.SecpkIncludes))
	for i, v := range t.SecpkIncludes {
		c.SecpkIncludes[i].v = v
	}
	return c.MarshalCBOR(w)
}

// this needs to be a struct or cborgen will peak into it and ignore the Unmarshal/Marshal functions
type messageIndices struct {
	v []uint64
}

func (t *messageIndices) UnmarshalCBOR(r io.Reader) (err error) {
	cr := cbg.NewCborReader(r)

	maj, extra, err := cr.ReadHeader()
	if err != nil {
		return err
	}

	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra > uint64(buildconstants.BlockMessageLimit) {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	if extra > 0 {
		t.v = make([]uint64, extra)
	}

	for i := 0; i < int(extra); i++ {
		maj, extra, err := cr.ReadHeader()
		if err != nil {
			return err
		}
		if maj != cbg.MajUnsignedInt {
			return fmt.Errorf("wrong type for uint64 field")
		}
		t.v[i] = extra

	}
	return nil
}

func (t *messageIndices) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}

	cw := cbg.NewCborWriter(w)

	if len(t.v) > buildconstants.BlockMessageLimit {
		return xerrors.Errorf("Slice value in field v was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajArray, uint64(len(t.v))); err != nil {
		return err
	}
	for _, v := range t.v {
		if err := cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, v); err != nil {
			return err
		}
	}
	return nil
}
