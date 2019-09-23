package rlepluslazy

import (
	"encoding/binary"

	"golang.org/x/xerrors"
)

func DecodeRLE(buf []byte) (RunIterator, error) {
	bv := readBitvec(buf)

	ver := bv.Get(2) // Read version
	if ver != Version {
		return nil, ErrWrongVersion
	}

	it := &rleIterator{bv: bv}

	// next run is previous in relation to prep
	// so we invert the value
	it.nextRun.Val = bv.Get(1) != 1
	if err := it.prep(); err != nil {
		return nil, err
	}
	return it, nil
}

type rleIterator struct {
	bv *rbitvec

	nextRun Run
}

func (it *rleIterator) HasNext() bool {
	return it.nextRun.Valid()
}

func (it *rleIterator) NextRun() (Run, error) {
	ret := it.nextRun
	return ret, it.prep()
}

func (it *rleIterator) prep() error {
	x := it.bv.Get(1)

	switch x {
	case 1:
		it.nextRun.Len = 1

	case 0:
		y := it.bv.Get(1)
		switch y {
		case 1:
			it.nextRun.Len = uint64(it.bv.Get(4))
		case 0:
			var buf = make([]byte, 0, 10)
			for {
				b := it.bv.Get(8)
				buf = append(buf, b)
				if b&0x80 == 0 {
					break
				}
				if len(buf) > 10 {
					return xerrors.Errorf("run too long: %w", ErrDecode)
				}
			}
			it.nextRun.Len, _ = binary.Uvarint(buf)
		}
	}

	it.nextRun.Val = !it.nextRun.Val
	return nil
}
