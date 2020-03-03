package sealmgr

import (
	"bytes"
	"context"
	"io"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
)


type SealTicket struct {
	Value abi.SealRandomness
	Epoch abi.ChainEpoch
}

type SealSeed struct {
	Value abi.InteractiveSealRandomness
	Epoch abi.ChainEpoch
}

func (st *SealTicket) Equals(ost *SealTicket) bool {
	return bytes.Equal(st.Value, ost.Value) && st.Epoch == ost.Epoch
}

func (st *SealSeed) Equals(ost *SealSeed) bool {
	return bytes.Equal(st.Value, ost.Value) && st.Epoch == ost.Epoch
}

// SectorInfo holds all sector-related metadata
type SectorInfo struct {
	ID abi.SectorID

	Pieces []abi.PieceInfo

	Ticket SealTicket
	Seed   SealSeed

	PreCommit1Out []byte

	Sealed   *cid.Cid
	Unsealed *cid.Cid

	CommitInput []byte
	Proof       []byte
}

func (si SectorInfo) PieceSizes() []abi.UnpaddedPieceSize {
	out := make([]abi.UnpaddedPieceSize, len(si.Pieces))
	for i := range out {
		out[i] = si.Pieces[i].Size.Unpadded()
	}

	return nil
}

type Worker interface {
	AddPiece(context.Context, SectorInfo, abi.UnpaddedPieceSize, io.Reader) (cid.Cid, SectorInfo, error)
	Run(context.Context, TaskType, SectorInfo) (SectorInfo, error)
}

type Manager interface {
	// NewSector allocates staging area for data
	NewSector() (SectorInfo, error)

	// AddPiece appends the piece to the specified sector. Returns PieceCID, and
	// mutated sector info
	//
	// Note: The passed reader can support other transfer mechanisms, making
	//  it possible to move the data between data transfer module and workers
	AddPiece(context.Context, SectorInfo, abi.UnpaddedPieceSize, io.Reader) (cid.Cid, SectorInfo, error)

	RunSeal(ctx context.Context, task TaskType, si SectorInfo) (SectorInfo, error)

	// Storage manager forwards proving calls
	sectorbuilder.Prover
}
