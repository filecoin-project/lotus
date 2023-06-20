package readerutil

import (
	"io"
	"os"
)

// NewReadSeekerFromReaderAt returns a new io.ReadSeeker from a io.ReaderAt.
// The returned io.ReadSeeker will read from the io.ReaderAt starting at the
// given base offset.
func NewReadSeekerFromReaderAt(readerAt io.ReaderAt, base int64) io.ReadSeeker {
	return &readSeekerFromReaderAt{
		readerAt: readerAt,
		base:     base,
		pos:      0,
	}
}

type readSeekerFromReaderAt struct {
	readerAt io.ReaderAt
	base     int64
	pos      int64
}

func (rs *readSeekerFromReaderAt) Read(p []byte) (n int, err error) {
	n, err = rs.readerAt.ReadAt(p, rs.pos+rs.base)
	rs.pos += int64(n)
	return n, err
}

func (rs *readSeekerFromReaderAt) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		rs.pos = offset
	case io.SeekCurrent:
		rs.pos += offset
	case io.SeekEnd:
		return 0, io.ErrUnexpectedEOF
	default:
		return 0, os.ErrInvalid
	}

	return rs.pos, nil
}
