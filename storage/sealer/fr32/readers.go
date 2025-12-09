package fr32

import (
	"io"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
)

// unpadReader implements an io.Reader that reads fr32-padded data from an
// underlying reader and returns unpadded data.
//
// Design: Similar to bufio.Reader, this uses a simple fill-and-read pattern:
// - padbuf: holds padded data read from the source
// - unpadbuf: holds unpadded data ready for the consumer
// - fill() reads padded data, unpads it, stores in unpadbuf
// - Read() copies from unpadbuf to the caller's buffer
type unpadReader struct {
	src io.Reader

	// padbuf holds padded data read from src (MUST be separate from unpadbuf)
	padbuf []byte
	// unpadbuf holds unpadded data ready for consumer
	unpadbuf []byte

	// r, w are read and write positions in unpadbuf
	// Data in unpadbuf[r:w] is available for reading
	r, w int

	// err stores any error from the underlying reader
	err error

	// left is how many padded bytes remain to read from src
	left uint64
}

// BufSize returns an appropriate buffer size for the given padded piece size.
// The returned size is suitable for use with NewUnpadReaderBuf.
func BufSize(sz abi.PaddedPieceSize) int {
	return int(MTTresh * mtChunkCount(sz))
}

// NewUnpadReader creates a new unpadding reader with an automatically sized buffer.
func NewUnpadReader(src io.Reader, sz abi.PaddedPieceSize) (io.Reader, error) {
	buf := make([]byte, BufSize(sz))
	return NewUnpadReaderBuf(src, sz, buf)
}

// NewUnpadReaderBuf creates a new unpadding reader using the provided buffer.
// sz is the number of padded bytes to read (must be a multiple of 128).
// buf is split 50/50: first half for padded input, second half for unpadded output.
// buf must be at least 256 bytes and a multiple of 128.
func NewUnpadReaderBuf(src io.Reader, sz abi.PaddedPieceSize, buf []byte) (io.Reader, error) {
	if sz%128 != 0 {
		return nil, xerrors.Errorf("padded size must be a multiple of 128: %d", sz)
	}

	if len(buf) < 256 {
		return nil, xerrors.Errorf("buffer too small: must be at least 256 bytes, got %d", len(buf))
	}

	if len(buf)%128 != 0 {
		return nil, xerrors.Errorf("buffer size must be a multiple of 128: %d", len(buf))
	}

	// Split buffer 50/50: first half for padded data, second half for unpadded output.
	// Round down to ensure padbuf is a multiple of 128.
	halfSize := (len(buf) / 2 / 128) * 128
	if halfSize < 128 {
		halfSize = 128
	}

	return &unpadReader{
		src:      src,
		padbuf:   buf[:halfSize],
		unpadbuf: buf[halfSize:],
		left:     uint64(sz),
	}, nil
}

// fill reads padded data from src, unpads it, and stores in unpadbuf.
func (r *unpadReader) fill() {
	// Slide existing data to beginning of buffer
	if r.r > 0 {
		copy(r.unpadbuf, r.unpadbuf[r.r:r.w])
		r.w -= r.r
		r.r = 0
	}

	// Check if we already have an error or no more data to read
	if r.err != nil {
		return
	}
	if r.left == 0 {
		r.err = io.EOF
		return
	}

	// Calculate how much padded data to read.
	// We need to ensure the unpadded result fits in remaining unpadbuf space.
	availSpace := len(r.unpadbuf) - r.w

	// Each 128 padded bytes produces 127 unpadded bytes
	maxChunks := availSpace / 127
	if maxChunks == 0 {
		return // Buffer too full to process even one chunk
	}

	// Clamp to what padbuf can hold
	padBufChunks := len(r.padbuf) / 128
	if maxChunks > padBufChunks {
		maxChunks = padBufChunks
	}

	// Clamp to what's left to read (in terms of declared size)
	toReadPadded := maxChunks * 128
	if uint64(toReadPadded) > r.left {
		toReadPadded = int(r.left)
		// Round down to complete chunks
		toReadPadded = (toReadPadded / 128) * 128
	}

	if toReadPadded == 0 {
		// Less than one full chunk remaining in declared size
		r.err = io.EOF
		return
	}

	// Read padded data from source
	n, err := io.ReadFull(r.src, r.padbuf[:toReadPadded])
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		// Partial or no data - process complete chunks only
		completeChunks := n / 128
		if completeChunks == 0 {
			r.err = io.EOF
			return
		}
		validPadded := completeChunks * 128
		r.left -= uint64(validPadded)

		// Unpad the complete chunks into unpadbuf starting at r.w
		unpadSize := completeChunks * 127
		Unpad(r.padbuf[:validPadded], r.unpadbuf[r.w:r.w+unpadSize])
		r.w += unpadSize
		r.err = io.EOF
		return
	}
	if err != nil {
		r.err = err
		return
	}

	// Successfully read toReadPadded bytes
	r.left -= uint64(n)

	// Unpad the data into unpadbuf starting at r.w
	chunks := n / 128
	unpadSize := chunks * 127
	Unpad(r.padbuf[:n], r.unpadbuf[r.w:r.w+unpadSize])
	r.w += unpadSize

	// If we've read everything from declared size, mark EOF for next fill
	if r.left == 0 {
		r.err = io.EOF
	}
}

func (r *unpadReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	// If buffer is empty, fill it
	if r.r == r.w {
		if r.err != nil {
			return 0, r.err
		}
		r.fill()
		// After fill, check again
		if r.r == r.w {
			return 0, r.err
		}
	}

	// Copy from buffer to p
	n = copy(p, r.unpadbuf[r.r:r.w])
	r.r += n
	return n, nil
}

// padWriter implements an io.WriteCloser that pads data with fr32 padding.
type padWriter struct {
	dst io.Writer

	stash []byte
	work  []byte
}

// NewPadWriter creates a new padding writer.
func NewPadWriter(dst io.Writer) io.WriteCloser {
	return &padWriter{
		dst: dst,
	}
}

func (w *padWriter) Write(p []byte) (int, error) {
	in := p

	if len(p)+len(w.stash) < 127 {
		w.stash = append(w.stash, p...)
		return len(p), nil
	}

	if len(w.stash) != 0 {
		in = append(w.stash, in...)
	}

	for {
		pieces := subPieces(abi.UnpaddedPieceSize(len(in)))
		biggest := pieces[len(pieces)-1]

		if abi.PaddedPieceSize(cap(w.work)) < biggest.Padded() {
			w.work = make([]byte, 0, biggest.Padded())
		}

		Pad(in[:int(biggest)], w.work[:int(biggest.Padded())])

		n, err := w.dst.Write(w.work[:int(biggest.Padded())])
		if err != nil {
			return int(abi.PaddedPieceSize(n).Unpadded()), err
		}

		in = in[biggest:]

		if len(in) < 127 {
			if cap(w.stash) < len(in) {
				w.stash = make([]byte, 0, len(in))
			}
			w.stash = w.stash[:len(in)]
			copy(w.stash, in)

			return len(p), nil
		}
	}
}

func (w *padWriter) Close() error {
	if len(w.stash) > 0 {
		return xerrors.Errorf("still have %d unprocessed bytes", len(w.stash))
	}

	// allow gc
	w.stash = nil
	w.work = nil
	w.dst = nil

	return nil
}
