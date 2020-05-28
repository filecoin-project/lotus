package ffiwrapper

import (
	"encoding/binary"
	"io"
	"os"

	"github.com/detailyang/go-fallocate"
	"golang.org/x/xerrors"

	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/storiface"
)

const veryLargeRle = 1 << 20

// Sectors can be partially unsealed. We support this by appending a small
// trailer to each unsealed sector file containing an RLE+ marking which bytes
// in a sector are unsealed, and which are not (holes)

// unsealed sector files internally have this structure
// [unpadded (raw) data][rle+][4B LE length fo the rle+ field]

type partialFile struct {
	maxPiece abi.PaddedPieceSize

	path      string
	allocated rlepluslazy.RLE

	file *os.File
}

func writeTrailer(maxPieceSize int64, w *os.File, r rlepluslazy.RunIterator) error {
	trailer, err := rlepluslazy.EncodeRuns(r, nil)
	if err != nil {
		return xerrors.Errorf("encoding trailer: %w", err)
	}

	// maxPieceSize == unpadded(sectorSize) == trailer start
	if _, err := w.Seek(maxPieceSize, io.SeekStart); err != nil {
		return xerrors.Errorf("seek to trailer start: %w", err)
	}

	rb, err := w.Write(trailer)
	if err != nil {
		return xerrors.Errorf("writing trailer data: %w", err)
	}

	if err := binary.Write(w, binary.LittleEndian, uint32(len(trailer))); err != nil {
		return xerrors.Errorf("writing trailer length: %w", err)
	}

	return w.Truncate(maxPieceSize + int64(rb) + 4)
}

func createPartialFile(maxPieceSize abi.PaddedPieceSize, path string) (*partialFile, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, xerrors.Errorf("openning partial file '%s': %w", path, err)
	}

	err = func() error {
		err := fallocate.Fallocate(f, 0, int64(maxPieceSize))
		if err != nil {
			return xerrors.Errorf("fallocate '%s': %w", path, err)
		}

		if err := writeTrailer(int64(maxPieceSize), f, &rlepluslazy.RunSliceIterator{}); err != nil {
			return xerrors.Errorf("writing trailer: %w", err)
		}

		return nil
	}()
	if err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, xerrors.Errorf("close empty partial file: %w", err)
	}

	return openPartialFile(maxPieceSize, path)
}

func openPartialFile(maxPieceSize abi.PaddedPieceSize, path string) (*partialFile, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return nil, xerrors.Errorf("openning partial file '%s': %w", path, err)
	}

	var rle rlepluslazy.RLE
	err = func() error {
		st, err := f.Stat()
		if err != nil {
			return xerrors.Errorf("stat '%s': %w", path, err)
		}
		if st.Size() < int64(maxPieceSize) {
			return xerrors.Errorf("sector file '%s' was smaller than the sector size %d < %d", path, st.Size(), maxPieceSize)
		}
		// read trailer
		var tlen [4]byte
		_, err = f.ReadAt(tlen[:], st.Size()-int64(len(tlen)))
		if err != nil {
			return xerrors.Errorf("reading trailer length: %w", err)
		}

		// sanity-check the length
		trailerLen := binary.LittleEndian.Uint32(tlen[:])
		expectLen := int64(trailerLen) + int64(len(tlen)) + int64(maxPieceSize)
		if expectLen != st.Size() {
			return xerrors.Errorf("file '%d' has inconsistent length; has %d bytes; expected %d (%d trailer, %d sector data)", path, st.Size(), expectLen, int64(trailerLen)+int64(len(tlen)), maxPieceSize)
		}
		if trailerLen > veryLargeRle {
			log.Warnf("Partial file '%s' has a VERY large trailer with %d bytes", path, trailerLen)
		}

		trailerStart := st.Size() - int64(len(tlen)) - int64(trailerLen)
		if trailerStart != int64(maxPieceSize) {
			return xerrors.Errorf("expected sector size to equal trailer start index")
		}

		trailerBytes := make([]byte, trailerLen)
		_, err = f.ReadAt(trailerBytes, trailerStart)
		if err != nil {
			return xerrors.Errorf("reading trailer: %w", err)
		}

		rle, err = rlepluslazy.FromBuf(trailerBytes)
		if err != nil {
			return xerrors.Errorf("decoding trailer: %w", err)
		}

		it, err := rle.RunIterator()
		if err != nil {
			return xerrors.Errorf("getting trailer run iterator: %w", err)
		}

		lastSet, err := rlepluslazy.LastIndex(it, true)
		if err != nil {
			return xerrors.Errorf("finding last set byte index: %w", err)
		}
		if lastSet > uint64(maxPieceSize) {
			return xerrors.Errorf("last set byte at index higher than sector size: %d > %d", lastSet, maxPieceSize)
		}

		return nil
	}()
	if err != nil {
		f.Close()
		return nil, err
	}

	return &partialFile{
		maxPiece:  maxPieceSize,
		path:      path,
		allocated: rle,
		file:      f,
	}, nil
}

func (pf *partialFile) Close() error {
	return pf.file.Close()
}

func (pf *partialFile) Writer(offset storiface.PaddedByteIndex, size abi.PaddedPieceSize) (io.Writer, error) {
	if _, err := pf.file.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, xerrors.Errorf("seek piece start: %w", err)
	}

	{
		have, err := pf.allocated.RunIterator()
		if err != nil {
			return nil, err
		}

		and, err := rlepluslazy.And(have, pieceRun(offset, size))
		if err != nil {
			return nil, err
		}

		c, err := rlepluslazy.Count(and)
		if err != nil {
			return nil, err
		}

		if c > 0 {
			log.Warnf("getting partial file writer overwriting %d allocated bytes", c)
		}
	}

	return pf.file, nil
}

func (pf *partialFile) MarkAllocated(offset storiface.PaddedByteIndex, size abi.PaddedPieceSize) error {
	have, err := pf.allocated.RunIterator()
	if err != nil {
		return err
	}

	ored, err := rlepluslazy.Or(have, pieceRun(offset, size))
	if err != nil {
		return err
	}

	if err := writeTrailer(int64(pf.maxPiece), pf.file, ored); err != nil {
		return xerrors.Errorf("writing trailer: %w", err)
	}

	return nil
}

func (pf *partialFile) Reader(offset storiface.PaddedByteIndex, size abi.PaddedPieceSize) (*os.File, error) {
	if _, err := pf.file.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, xerrors.Errorf("seek piece start: %w", err)
	}

	{
		have, err := pf.allocated.RunIterator()
		if err != nil {
			return nil, err
		}

		and, err := rlepluslazy.And(have, pieceRun(offset, size))
		if err != nil {
			return nil, err
		}

		c, err := rlepluslazy.Count(and)
		if err != nil {
			return nil, err
		}

		if c != uint64(size) {
			log.Warnf("getting partial file reader reading %d unallocated bytes", uint64(size)-c)
		}
	}

	return pf.file, nil
}

func (pf *partialFile) Allocated() (rlepluslazy.RunIterator, error) {
	return pf.allocated.RunIterator()
}

func pieceRun(offset storiface.PaddedByteIndex, size abi.PaddedPieceSize) rlepluslazy.RunIterator {
	var runs []rlepluslazy.Run
	if offset > 0 {
		runs = append(runs, rlepluslazy.Run{
			Val: false,
			Len: uint64(offset),
		})
	}

	runs = append(runs, rlepluslazy.Run{
		Val: true,
		Len: uint64(size),
	})

	return &rlepluslazy.RunSliceIterator{Runs: runs}
}
