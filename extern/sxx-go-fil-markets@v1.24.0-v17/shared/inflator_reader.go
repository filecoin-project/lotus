package shared

import (
	"io"
	"sync"

	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
)

// ReadSeekStarter implements io.Reader and allows the caller to seek to
// the start of the reader
type ReadSeekStarter interface {
	io.Reader
	SeekStart() error
}

// inflatorReader wraps the MultiReader returned by padreader so that we can
// add a SeekStart method. It's used for example when there is an error
// reading from the reader and we need to return to the start.
type inflatorReader struct {
	readSeeker  io.ReadSeeker
	payloadSize uint64
	targetSize  abi.UnpaddedPieceSize

	lk           sync.RWMutex
	paddedReader io.Reader
}

var _ ReadSeekStarter = (*inflatorReader)(nil)

func NewInflatorReader(readSeeker io.ReadSeeker, payloadSize uint64, targetSize abi.UnpaddedPieceSize) (*inflatorReader, error) {
	paddedReader, err := padreader.NewInflator(readSeeker, payloadSize, targetSize)
	if err != nil {
		return nil, err
	}

	return &inflatorReader{
		readSeeker:   readSeeker,
		paddedReader: paddedReader,
		payloadSize:  payloadSize,
		targetSize:   targetSize,
	}, nil
}

func (r *inflatorReader) Read(p []byte) (n int, err error) {
	r.lk.RLock()
	defer r.lk.RUnlock()

	return r.paddedReader.Read(p)
}

func (r *inflatorReader) SeekStart() error {
	r.lk.Lock()
	defer r.lk.Unlock()

	// Seek to the start of the underlying reader
	_, err := r.readSeeker.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	// Recreate the padded reader
	paddedReader, err := padreader.NewInflator(r.readSeeker, r.payloadSize, r.targetSize)
	if err != nil {
		return err
	}
	r.paddedReader = paddedReader

	return nil
}
