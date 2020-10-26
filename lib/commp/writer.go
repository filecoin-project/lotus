package commp

import (
	"bytes"
	"math/bits"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/zerocomm"
)

const commPBufPad = abi.PaddedPieceSize(8 << 20)
const CommPBuf = abi.UnpaddedPieceSize(commPBufPad - (commPBufPad / 128)) // can't use .Unpadded() for const

type Writer struct {
	len    int64
	buf    [CommPBuf]byte
	leaves []cid.Cid
}

func (w *Writer) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		buffered := int(w.len % int64(len(w.buf)))
		toBuffer := len(w.buf) - buffered
		if toBuffer > len(p) {
			toBuffer = len(p)
		}

		copied := copy(w.buf[buffered:], p[:toBuffer])
		p = p[copied:]
		w.len += int64(copied)

		if copied > 0 && w.len%int64(len(w.buf)) == 0 {
			leaf, err := ffiwrapper.GeneratePieceCIDFromFile(abi.RegisteredSealProof_StackedDrg32GiBV1, bytes.NewReader(w.buf[:]), CommPBuf)
			if err != nil {
				return 0, err
			}
			w.leaves = append(w.leaves, leaf)
		}
	}
	return n, nil
}

func (w *Writer) Sum() (api.DataCIDSize, error) {
	// process last non-zero leaf if exists
	lastLen := w.len % int64(len(w.buf))
	rawLen := w.len

	// process remaining bit of data
	if lastLen != 0 {
		if len(w.leaves) != 0 {
			copy(w.buf[lastLen:], make([]byte, int(int64(CommPBuf)-lastLen)))
			lastLen = int64(CommPBuf)
		}

		r, sz := padreader.New(bytes.NewReader(w.buf[:lastLen]), uint64(lastLen))
		p, err := ffiwrapper.GeneratePieceCIDFromFile(abi.RegisteredSealProof_StackedDrg32GiBV1, r, sz)
		if err != nil {
			return api.DataCIDSize{}, err
		}

		if sz < CommPBuf { // special case for pieces smaller than 16MiB
			return api.DataCIDSize{
				PayloadSize: w.len,
				PieceSize:   sz.Padded(),
				PieceCID:    p,
			}, nil
		}

		w.leaves = append(w.leaves, p)
	}

	// pad with zero pieces to power-of-two size
	fillerLeaves := (1 << (bits.Len(uint(len(w.leaves) - 1)))) - len(w.leaves)
	for i := 0; i < fillerLeaves; i++ {
		w.leaves = append(w.leaves, zerocomm.ZeroPieceCommitment(CommPBuf))
	}

	if len(w.leaves) == 1 {
		return api.DataCIDSize{
			PayloadSize: rawLen,
			PieceSize:   abi.PaddedPieceSize(len(w.leaves)) * commPBufPad,
			PieceCID:    w.leaves[0],
		}, nil
	}

	pieces := make([]abi.PieceInfo, len(w.leaves))
	for i, leaf := range w.leaves {
		pieces[i] = abi.PieceInfo{
			Size:     commPBufPad,
			PieceCID: leaf,
		}
	}

	p, err := ffi.GenerateUnsealedCID(abi.RegisteredSealProof_StackedDrg32GiBV1, pieces)
	if err != nil {
		return api.DataCIDSize{}, xerrors.Errorf("generating unsealed CID: %w", err)
	}

	return api.DataCIDSize{
		PayloadSize: rawLen,
		PieceSize:   abi.PaddedPieceSize(len(w.leaves)) * commPBufPad,
		PieceCID:    p,
	}, nil
}
