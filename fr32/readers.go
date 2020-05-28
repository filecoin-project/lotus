package fr32

import (
	"io"
	"math/bits"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
)

type padReader struct {
	src io.Reader

	left uint64
	work []byte
}

func NewPadReader(src io.Reader, sz abi.UnpaddedPieceSize) (io.Reader, error) {
	if err := sz.Validate(); err != nil {
		return nil, xerrors.Errorf("bad piece size: %w", err)
	}

	buf := make([]byte, mtTresh*mtChunkCount(sz.Padded()))

	return &padReader{
		src: src,

		left: uint64(sz.Padded()),
		work: buf,
	}, nil
}

func (r *padReader) Read(out []byte) (int, error) {
	if r.left == 0 {
		return 0, io.EOF
	}

	outTwoPow := 1 << (63 - bits.LeadingZeros64(uint64(len(out))))

	if err := abi.PaddedPieceSize(outTwoPow).Validate(); err != nil {
		return 0, xerrors.Errorf("output must be of valid padded piece size: %w", err)
	}

	todo := abi.PaddedPieceSize(outTwoPow).Unpadded()
	if r.left < uint64(todo.Padded()) {
		todo = abi.PaddedPieceSize(1 << (63 - bits.LeadingZeros64(r.left))).Unpadded()
	}

	r.left -= uint64(todo.Padded())

	n, err := r.src.Read(r.work[:todo])
	if err != nil && err != io.EOF {
		return n, err
	}

	Pad(r.work[:todo], out[:todo.Padded()])

	return int(todo.Padded()), err
}

func NewPadWriter(dst io.Writer, sz abi.UnpaddedPieceSize) (io.Writer, error) {
	if err := sz.Validate(); err != nil {
		return nil, xerrors.Errorf("bad piece size: %w", err)
	}

	buf := make([]byte, mtTresh*mtChunkCount(sz.Padded()))

	// TODO: Real writer
	r, w := io.Pipe()

	pr, err := NewPadReader(r, sz)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			n, err := pr.Read(buf)
			if err != nil && err != io.EOF {
				r.CloseWithError(err)
				return
			}

			if _, err := dst.Write(buf[:n]); err != nil {
				r.CloseWithError(err)
				return
			}
		}
	}()

	return w, err
}

type unpadReader struct {
	src io.Reader

	left uint64
	work []byte
}

func NewUnpadReader(src io.Reader, sz abi.PaddedPieceSize) (io.Reader, error) {
	if err := sz.Validate(); err != nil {
		return nil, xerrors.Errorf("bad piece size: %w", err)
	}

	buf := make([]byte, mtTresh*mtChunkCount(sz))

	return &unpadReader{
		src: src,

		left: uint64(sz),
		work: buf,
	}, nil
}

func (r *unpadReader) Read(out []byte) (int, error) {
	if r.left == 0 {
		return 0, io.EOF
	}

	chunks := len(out) / 127

	outTwoPow := 1 << (63 - bits.LeadingZeros64(uint64(chunks*128)))

	if err := abi.PaddedPieceSize(outTwoPow).Validate(); err != nil {
		return 0, xerrors.Errorf("output must be of valid padded piece size: %w", err)
	}

	todo := abi.PaddedPieceSize(outTwoPow)
	if r.left < uint64(todo) {
		todo = abi.PaddedPieceSize(1 << (63 - bits.LeadingZeros64(r.left)))
	}

	r.left -= uint64(todo)

	n, err := r.src.Read(r.work[:todo])
	if err != nil && err != io.EOF {
		return n, err
	}

	if n != int(todo) {
		return 0, xerrors.Errorf("didn't read enough: %w", err)
	}

	Unpad(r.work[:todo], out[:todo.Unpadded()])

	return int(todo.Unpadded()), err
}
