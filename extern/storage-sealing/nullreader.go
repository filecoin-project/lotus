package sealing

import (
	"io"

	"github.com/filecoin-project/specs-actors/actors/abi"
	nr "github.com/filecoin-project/storage-fsm/lib/nullreader"
)

type NullReader struct {
	*io.LimitedReader
}

func NewNullReader(size abi.UnpaddedPieceSize) io.Reader {
	return &NullReader{(io.LimitReader(&nr.Reader{}, int64(size))).(*io.LimitedReader)}
}

func (m NullReader) NullBytes() int64 {
	return m.N
}
