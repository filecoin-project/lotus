package fr32

import (
	"io"
	"math/bits"

	pool "github.com/libp2p/go-buffer-pool"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
)

type unpadReader struct {
	src io.Reader

	left uint64
	work []byte

	stash []byte
}

func BufSize(sz abi.PaddedPieceSize) int {
	return int(MTTresh * mtChunkCount(sz))
}

func NewUnpadReader(src io.Reader, sz abi.PaddedPieceSize) (io.Reader, error) {
	buf := make([]byte, BufSize(sz))

	return NewUnpadReaderBuf(src, sz, buf)
}

func NewUnpadReaderBuf(src io.Reader, sz abi.PaddedPieceSize, buf []byte) (io.Reader, error) {
	if err := sz.Validate(); err != nil {
		return nil, xerrors.Errorf("bad piece size: %w", err)
	}

	if abi.PaddedPieceSize(len(buf)).Validate() != nil {
		return nil, xerrors.Errorf("bad buffer size")
	}

	return &unpadReader{
		src: src,

		left: uint64(sz),
		work: buf,
	}, nil
}

func (r *unpadReader) Read(out []byte) (int, error) {
	idealReadSize := abi.PaddedPieceSize(len(r.work)).Unpadded()

	var err error
	var rn int
	if len(r.stash) == 0 && len(out) < int(idealReadSize) {
		r.stash = pool.Get(int(idealReadSize))

		rn, err = r.readInner(r.stash)
		r.stash = r.stash[:rn]
	}

	if len(r.stash) > 0 {
		n := copy(out, r.stash)
		r.stash = r.stash[n:]

		if len(r.stash) == 0 {
			pool.Put(r.stash)
			r.stash = nil
		}

		if err == io.EOF && rn > n {
			err = nil
		}

		return n, err
	}

	return r.readInner(out)
}

// readInner reads from the underlying reader into the provided buffer.
// It requires that out[] is padded(power-of-two).unpadded()-sized, ideally quite large.
func (r *unpadReader) readInner(out []byte) (int, error) {
	if r.left == 0 {
		return 0, io.EOF
	}

	chunks := len(out) / 127

	outTwoPow := 1 << (63 - bits.LeadingZeros64(uint64(chunks*128)))

	if err := abi.PaddedPieceSize(outTwoPow).Validate(); err != nil {
		return 0, xerrors.Errorf("output must be of valid padded piece size: %w", err)
	}

	// Clamp `todo` to the length of the work buffer to prevent buffer overflows
	todo := min(abi.PaddedPieceSize(outTwoPow), abi.PaddedPieceSize(len(r.work)))
	if r.left < uint64(todo) {
		todo = abi.PaddedPieceSize(1 << (63 - bits.LeadingZeros64(r.left)))
	}

	r.left -= uint64(todo)

	n, err := io.ReadAtLeast(r.src, r.work[:todo], int(todo))
	if err == io.ErrUnexpectedEOF {
		// We got a partial read. This happens when the underlying reader
		// doesn't have as much data as expected (e.g., non-power-of-2 pieces).
		// Process what we got.
		if n > 0 {
			// Round down to complete 128-byte chunks
			completeChunks := n / 128
			if completeChunks > 0 {
				validBytes := completeChunks * 128
				Unpad(r.work[:validBytes], out[:completeChunks*127])
				// Adjust left to reflect that we couldn't read everything
				r.left = 0
				return completeChunks * 127, io.EOF
			}
		}
		// Not enough data for even one chunk
		return 0, io.EOF
	}
	if err == io.EOF {
		// Clean EOF with no data
		if n == 0 {
			return 0, io.EOF
		}
		// Got some data with EOF - shouldn't happen with ReadAtLeast but handle it
		return 0, xerrors.Errorf("unexpected EOF with partial read: %d bytes", n)
	}
	if err != nil {
		return 0, err
	}
	if n < int(todo) {
		return 0, xerrors.Errorf("short read without EOF: got %d, expected %d", n, todo)
	}

	Unpad(r.work[:todo], out[:todo.Unpadded()])

	return int(todo.Unpadded()), err
}

type padWriter struct {
	dst io.Writer

	stash []byte
	work  []byte
}

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
