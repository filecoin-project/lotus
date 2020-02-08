package storage

import (
	"context"
	"io"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage/sealing"
)

// TODO: refactor this to be direct somehow

func (m *Miner) AllocatePiece(size uint64) (sectorID abi.SectorNumber, offset uint64, err error) {
	return m.sealing.AllocatePiece(size)
}

func (m *Miner) SealPiece(ctx context.Context, size abi.UnpaddedPieceSize, r io.Reader, sectorID abi.SectorNumber, dealID abi.DealID) error {
	return m.sealing.SealPiece(ctx, size, r, sectorID, dealID)
}

func (m *Miner) ListSectors() ([]sealing.SectorInfo, error) {
	return m.sealing.ListSectors()
}

func (m *Miner) GetSectorInfo(sid uint64) (sealing.SectorInfo, error) {
	return m.sealing.GetSectorInfo(sid)
}

func (m *Miner) PledgeSector() error {
	return m.sealing.PledgeSector()
}

func (m *Miner) ForceSectorState(ctx context.Context, id uint64, state api.SectorState) error {
	return m.sealing.ForceSectorState(ctx, id, state)
}
