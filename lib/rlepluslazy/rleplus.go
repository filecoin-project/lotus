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
}

func FromBuf(buf []byte) (RLE, error) {
	rle := RLE{buf: buf}

	if len(buf) > 0 && buf[0]&3 != Version {
		return RLE{}, xerrors.Errorf("could not create RLE+ for a buffer: %w", ErrWrongVersion)
	}

	_, err := rle.Count()
	if err != nil {
		return RLE{}, err
	}

	return rle, nil
}

func (rle *RLE) RunIterator() (RunIterator, error) {
	source, err := DecodeRLE(rle.buf)
	return source, err
}

func (rle *RLE) Count() (uint64, error) {
	it, err := rle.RunIterator()
	if err != nil {
		return 0, err
	}
	return Count(it)
}
