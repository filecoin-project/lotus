package sealing

import (
	"context"
	"io"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"

	nr "github.com/filecoin-project/storage-fsm/lib/nullreader"
)

func (m *Sealing) pledgeReader(size abi.UnpaddedPieceSize) io.Reader {
	return io.LimitReader(&nr.Reader{}, int64(size))
}

func (m *Sealing) pledgeSector(ctx context.Context, sectorID abi.SectorID, existingPieceSizes []abi.UnpaddedPieceSize, sizes ...abi.UnpaddedPieceSize) ([]abi.PieceInfo, error) {
	if len(sizes) == 0 {
		return nil, nil
	}

	log.Infof("Pledge %d, contains %+v", sectorID, existingPieceSizes)

	out := make([]abi.PieceInfo, len(sizes))
	for i, size := range sizes {
		ppi, err := m.sealer.AddPiece(ctx, sectorID, existingPieceSizes, size, m.pledgeReader(size))
		if err != nil {
			return nil, xerrors.Errorf("add piece: %w", err)
		}

		existingPieceSizes = append(existingPieceSizes, size)

		out[i] = ppi
	}

	return out, nil
}

func (m *Sealing) PledgeSector() error {
	go func() {
		ctx := context.TODO() // we can't use the context from command which invokes
		// this, as we run everything here async, and it's cancelled when the
		// command exits

		size := abi.PaddedPieceSize(m.sealer.SectorSize()).Unpadded()

		_, rt, err := ffiwrapper.ProofTypeFromSectorSize(m.sealer.SectorSize())
		if err != nil {
			log.Error(err)
			return
		}

		sid, err := m.sc.Next()
		if err != nil {
			log.Errorf("%+v", err)
			return
		}
		err = m.sealer.NewSector(ctx, m.minerSector(sid))
		if err != nil {
			log.Errorf("%+v", err)
			return
		}

		pieces, err := m.pledgeSector(ctx, m.minerSector(sid), []abi.UnpaddedPieceSize{}, size)
		if err != nil {
			log.Errorf("%+v", err)
			return
		}

		pdis := make([]Piece, len(pieces))
		for idx := range pdis {
			pdis[idx] = Piece{
				Piece:    pieces[idx],
				DealInfo: nil,
			}
		}

		if err := m.newSector(sid, rt, pdis); err != nil {
			log.Errorf("%+v", err)
			return
		}
	}()
	return nil
}
