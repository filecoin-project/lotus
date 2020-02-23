package sealing

import (
	"context"
	"io"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
)

type nullReader struct {}

func (nullReader) Read(out []byte) (int, error) {
	for i := range out {
		out[i] = 0
	}
	return len(out), nil
}

func (m *Sealing) pledgeReader(size abi.UnpaddedPieceSize) io.Reader {
	return io.LimitReader(&nullReader{}, int64(size))
}

func (m *Sealing) pledgeSector(ctx context.Context, sectorID abi.SectorNumber, existingPieceSizes []abi.UnpaddedPieceSize, sizes ...abi.UnpaddedPieceSize) ([]Piece, error) {
	if len(sizes) == 0 {
		return nil, nil
	}

	log.Infof("Pledge %d, contains %+v", sectorID, existingPieceSizes)

	out := make([]Piece, len(sizes))
	for i, size := range sizes {
		ppi, err := m.sb.AddPiece(ctx, size, sectorID, m.pledgeReader(size), existingPieceSizes)
		if err != nil {
			return nil, xerrors.Errorf("add piece: %w", err)
		}

		existingPieceSizes = append(existingPieceSizes, size)

		out[i] = Piece{
			Size:  ppi.Size,
			CommP: ppi.CommP[:],
		}
	}

	return out, nil
}

func (m *Sealing) PledgeSector() error {
	go func() {
		ctx := context.TODO() // we can't use the context from command which invokes
		// this, as we run everything here async, and it's cancelled when the
		// command exits

		size := abi.PaddedPieceSize(m.sb.SectorSize()).Unpadded()

		sid, err := m.sb.AcquireSectorNumber()
		if err != nil {
			log.Errorf("%+v", err)
			return
		}

		pieces, err := m.pledgeSector(ctx, sid, []abi.UnpaddedPieceSize{}, size)
		if err != nil {
			log.Errorf("%+v", err)
			return
		}

		if err := m.newSector(sid, pieces); err != nil {
			log.Errorf("%+v", err)
			return
		}
	}()
	return nil
}
