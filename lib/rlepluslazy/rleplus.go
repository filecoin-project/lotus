package rlepluslazy

import (
	"errors"
	"fmt"

	"golang.org/x/xerrors"
)

const Version = 0

var (
	ErrWrongVersion = errors.New("invalid RLE+ version")
	ErrDecode       = fmt.Errorf("invalid encoding for RLE+ version %d", Version)
)

type RLE struct {
	buf []byte

	changes []change
}

type change struct {
	set   bool
	index uint64
}

func FromBuf(buf []byte) (*RLE, error) {
	rle := &RLE{buf: buf}

	if len(buf) > 0 && buf[0]&3 != Version {
		return nil, xerrors.Errorf("could not create RLE+ for a buffer: %w", ErrWrongVersion)
	}

	return rle, nil
}

func (rle *RLE) RunIterator() (RunIterator, error) {
	return DecodeRLE(rle.buf)
}

func (rle *RLE) Set(index uint64) {
	rle.changes = append(rle.changes, change{set: true, index: index})
}

func (rle *RLE) Clear(index uint64) {
	rle.changes = append(rle.changes, change{set: false, index: index})
}
