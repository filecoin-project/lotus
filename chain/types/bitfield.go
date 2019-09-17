package types

import (
	"fmt"
	"io"

	"github.com/filecoin-project/go-lotus/extern/rleplus"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

type BitField struct {
	bits map[uint64]struct{}
}

// Set ...s bit in the BitField
func (bf BitField) Set(bit uint64) {
	bf.bits[bit] = struct{}{}
}

// Clear ...s bit in the BitField
func (bf BitField) Clear(bit uint64) {
	delete(bf.bits, bit)
}

// Has checkes if bit is set in the BitField
func (bf BitField) Has(bit uint64) bool {
	_, ok := bf.bits[bit]
	return ok
}

func (bf BitField) MarshalCBOR(w io.Writer) error {
	ints := make([]uint64, 0, len(bf.bits))
	for i := range bf.bits {
		ints = append(ints, i)
	}

	rle, _, err := rleplus.Encode(ints) // Encode sorts internally
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

	rle := make([]byte, extra)
	if _, err := io.ReadFull(br, rle); err != nil {
		return err
	}

	ints, err := rleplus.Decode(rle)
	if err != nil {
		return xerrors.Errorf("could not decode rle+: %w", err)
	}
	bf.bits = make(map[uint64]struct{})
	for _, i := range ints {
		bf.bits[i] = struct{}{}
	}

	return nil
}
