package sealmgr

import (
	"context"
	"io"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
)

type Worker interface {
	sectorbuilder.Sealer
	sectorbuilder.Prover
}

type Manager interface {
	SectorSize() abi.SectorSize

	// NewSector allocates staging area for data
	// Storage manager forwards proof-related calls
	NewSector() (abi.SectorNumber, error)

	// TODO: Can[Pre]Commit[1,2]
	// TODO: Scrub() []Faults

	// TODO: Separate iface
	ReadPieceFromSealedSector(context.Context, abi.SectorNumber, sectorbuilder.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error)

	sectorbuilder.Sealer
	sectorbuilder.Prover
}
