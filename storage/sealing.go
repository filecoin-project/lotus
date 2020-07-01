package storage

import (
	"context"
	"io"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"

	sealing "github.com/filecoin-project/storage-fsm"
)

// TODO: refactor this to be direct somehow

func (m *Miner) Address() address.Address {
	return m.sealing.Address()
}

func (m *Miner) AllocatePiece(size abi.UnpaddedPieceSize) (sectorID abi.SectorNumber, offset uint64, err error) {
	return m.sealing.AllocatePiece(size)
}

func (m *Miner) SealPiece(ctx context.Context, size abi.UnpaddedPieceSize, r io.Reader, sectorID abi.SectorNumber, d sealing.DealInfo) error {
	return m.sealing.SealPiece(ctx, size, r, sectorID, d)
}

func (m *Miner) ListSectors() ([]sealing.SectorInfo, error) {
	return m.sealing.ListSectors()
}

func (m *Miner) GetSectorInfo(sid abi.SectorNumber) (sealing.SectorInfo, error) {
	return m.sealing.GetSectorInfo(sid)
}

func (m *Miner) PledgeSector() error {
	return m.sealing.PledgeSector()
}

func (m *Miner) ForceSectorState(ctx context.Context, id abi.SectorNumber, state sealing.SectorState) error {
	return m.sealing.ForceSectorState(ctx, id, state)
}

func (m *Miner) RemoveSector(ctx context.Context, id abi.SectorNumber) error {
	return m.sealing.Remove(ctx, id)
}
