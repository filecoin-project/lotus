package sealmgr

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"
)

type Worker interface {
	sectorbuilder.Sealer
	storage.Prover
}

type Manager interface {
	SectorSize() abi.SectorSize

	// TODO: Can[Pre]Commit[1,2]
	// TODO: Scrub() []Faults

	// TODO: Separate iface
	ReadPieceFromSealedSector(context.Context, abi.SectorID, sectorbuilder.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error)

	sectorbuilder.Sealer
	storage.Prover
}
