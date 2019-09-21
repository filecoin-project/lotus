package rlepluslazy

import (
	"encoding/binary"
	"errors"
	"fmt"

	bitvector "github.com/filecoin-project/go-lotus/lib/rlepluslazy/internal"
	"golang.org/x/xerrors"
)

const Version = 0

var (
	ErrWrongVersion = errors.New("invalid RLE+ version")
	ErrDecode       = fmt.Errorf("invalid encoding for RLE+ version %d", Version)
)

type RLE struct {
	vec *bitvector.BitVector
}

func FromBuf(buf []byte) (*RLE, error) {
	rle := &RLE{vec: bitvector.NewBitVector(buf, bitvector.LSB0)}

	if err := rle.check(); err != nil {
		return nil, xerrors.Errorf("could not create RLE+ for a buffer: %w", err)
	}
	return rle, nil
}

func (rle *RLE) check() error {
	ver := rle.vec.Take(0, 2, bitvector.LSB0)
	if ver != Version {
		return ErrWrongVersion
	}
	return nil
}

func (rle *RLE) RunIterator() (RunIterator, error) {
	vit := rle.vec.Iterator(bitvector.LSB0)
	vit(2) // Take version

	it := &rleIterator{next: vit}
	// next run is previous in relation to prep
	// so we invert the value
	it.nextRun.Val = vit(1) != 1
	if err := it.prep(); err != nil {
		return nil, err
	}
	return it, nil
}

type rleIterator struct {
	next func(uint) byte

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
	x := it.next(1)

	switch x {
	case 1:
		it.nextRun.Len = 1

	case 0:
		y := it.next(1)
		switch y {
		case 1:
			it.nextRun.Len = uint64(it.next(4))
		case 0:
			var buf = make([]byte, 0, 10)
			for {
				b := it.next(8)
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

func (rle *RLE) Iterator() (*iterator, error) {
	vit := rle.vec.Iterator(bitvector.LSB0)
	vit(2) // Take version

	it := &iterator{next: vit}
	if err := it.prep(vit(1)); err != nil {
		return nil, err
	}
	return it, nil
}

type iterator struct {
	next func(uint) byte

	curIdx uint64
	rep    uint64
}

func (it *iterator) HasNext() bool {
	return it.rep != 0
}

func (it *iterator) prep(curBit byte) error {

loop:
	for it.rep == 0 {
		x := it.next(1)
		switch x {
		case 1:
			it.rep = 1
		case 0:
			y := it.next(1)
			switch y {
			case 1:
				it.rep = uint64(it.next(4))
			case 0:
				var buf = make([]byte, 0, 10)
				for {
					b := it.next(8)
					buf = append(buf, b)
					if b&0x80 == 0 {
						break
					}
					if len(buf) > 10 {
						return xerrors.Errorf("run too long: %w", ErrDecode)
					}
				}
				it.rep, _ = binary.Uvarint(buf)
			}

			// run with 0 length means end
			if it.rep == 0 {
				break loop
			}
		}

		if curBit == 0 {
			curBit = 1
			it.curIdx = it.curIdx + it.rep
			it.rep = 0
		}
	}
	return nil
}

func (it *iterator) Next() (uint64, error) {
	it.rep--
	res := it.curIdx
	it.curIdx++
	return res, it.prep(0)
}
