package types

import (
	"fmt"
	"io"

	rlepluslazy "github.com/filecoin-project/lotus/lib/rlepluslazy"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

type BitField struct {
	rle rlepluslazy.RLE

	bits map[uint64]struct{}
}

func NewBitField() BitField {
	rle, _ := rlepluslazy.FromBuf([]byte{})
	return BitField{
		rle:  rle,
		bits: make(map[uint64]struct{}),
	}
}

func BitFieldFromSet(setBits []uint64) BitField {
	res := BitField{bits: make(map[uint64]struct{})}
	for _, b := range setBits {
		res.bits[b] = struct{}{}
	}
	return res
}

func (bf BitField) sum() (rlepluslazy.RunIterator, error) {
	if len(bf.bits) == 0 {
		return bf.rle.RunIterator()
	}

	a, err := bf.rle.RunIterator()
	if err != nil {
		return nil, err
	}
	slc := make([]uint64, 0, len(bf.bits))
	for b := range bf.bits {
		slc = append(slc, b)
	}

	b, err := rlepluslazy.RunsFromSlice(slc)
	if err != nil {
		return nil, err
	}

	res, err := rlepluslazy.Sum(a, b)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Set ...s bit in the BitField
func (bf BitField) Set(bit uint64) {
	bf.bits[bit] = struct{}{}
}

func (bf BitField) Count() (uint64, error) {
	s, err := bf.sum()
	if err != nil {
		return 0, err
	}
	return rlepluslazy.Count(s)
}

// All returns all set bits, in random order
func (bf BitField) All() ([]uint64, error) {

	runs, err := bf.sum()
	if err != nil {
		return nil, err
	}

	res, err := rlepluslazy.SliceFromRuns(runs)
	if err != nil {
		return nil, err
	}

	return res, err
}

func (bf BitField) MarshalCBOR(w io.Writer) error {
	ints := make([]uint64, 0, len(bf.bits))
	for i := range bf.bits {
		ints = append(ints, i)
	}

	s, err := bf.sum()
	if err != nil {
		return err
	}

	rle, err := rlepluslazy.EncodeRuns(s, []byte{})
	if err != nil {
		return err
	}

	if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajByteString, uint64(len(rle)))); err != nil {
		return err
	}
	if _, err = w.Write(rle); err != nil {
		return xerrors.Errorf("writing rle: %w", err)
	}
	return nil
}

func (bf *BitField) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if extra > 8192 {
		return fmt.Errorf("array too large")
	}

	if maj != cbg.MajByteString {
		return fmt.Errorf("expected byte array")
	}

	buf := make([]byte, extra)
	if _, err := io.ReadFull(br, buf); err != nil {
		return err
	}

	rle, err := rlepluslazy.FromBuf(buf)
	if err != nil {
		return xerrors.Errorf("could not decode rle+: %w", err)
	}
	bf.rle = rle
	bf.bits = make(map[uint64]struct{})

	return nil
}
