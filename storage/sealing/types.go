package sealing

import (
	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
)

type SealTicket struct {
	BlockHeight uint64
	TicketBytes []byte
}

func (t *SealTicket) SB() sectorbuilder.SealTicket {
	out := sectorbuilder.SealTicket{BlockHeight: t.BlockHeight}
	copy(out.TicketBytes[:], t.TicketBytes)
	return out
}

type SealSeed struct {
	BlockHeight abi.ChainEpoch
	TicketBytes []byte
}

func (t *SealSeed) SB() sectorbuilder.SealSeed {
	out := sectorbuilder.SealSeed{BlockHeight: uint64(t.BlockHeight)}
	copy(out.TicketBytes[:], t.TicketBytes)
	return out
}

func (t *SealSeed) Equals(o *SealSeed) bool {
	return string(t.TicketBytes) == string(o.TicketBytes) && t.BlockHeight == o.BlockHeight
}

type Piece struct {
	DealID abi.DealID

	Size  abi.UnpaddedPieceSize
	CommP []byte
}

func (p *Piece) ppi() (out sectorbuilder.PublicPieceInfo) {
	out.Size = uint64(p.Size)
	copy(out.CommP[:], p.CommP)
	return out
}

type Log struct {
	Timestamp uint64
	Trace     string // for errors

	Message string

	// additional data (Event info)
	Kind string
}

type SectorInfo struct {
	State    api.SectorState
	SectorID abi.SectorNumber
	Nonce    uint64 // TODO: remove

	// Packing

	Pieces []Piece

	// PreCommit
	CommD  []byte
	CommR  []byte
	Proof  []byte
	Ticket SealTicket

	PreCommitMessage *cid.Cid

	// WaitSeed
	Seed SealSeed

	// Committing
	CommitMessage *cid.Cid

	// Faults
	FaultReportMsg *cid.Cid

	// Debug
	LastErr string

	Log []Log
}

func (t *SectorInfo) pieceInfos() []sectorbuilder.PublicPieceInfo {
	out := make([]sectorbuilder.PublicPieceInfo, len(t.Pieces))
	for i, piece := range t.Pieces {
		out[i] = piece.ppi()
	}
	return out
}

func (t *SectorInfo) deals() []abi.DealID {
	out := make([]abi.DealID, len(t.Pieces))
	for i, piece := range t.Pieces {
		out[i] = piece.DealID
	}
	return out
}

func (t *SectorInfo) existingPieces() []abi.UnpaddedPieceSize {
	out := make([]abi.UnpaddedPieceSize, len(t.Pieces))
	for i, piece := range t.Pieces {
		out[i] = piece.Size
	}
	return out
}

func (t *SectorInfo) rspco() sectorbuilder.RawSealPreCommitOutput {
	var out sectorbuilder.RawSealPreCommitOutput

	copy(out.CommD[:], t.CommD)
	copy(out.CommR[:], t.CommR)

	return out
}
