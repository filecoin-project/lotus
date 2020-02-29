package sealing

import (
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
)

type Piece struct {
	DealID *abi.DealID

	Size  abi.UnpaddedPieceSize
	CommP cid.Cid
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

	SectorType abi.RegisteredProof

	// Packing

	Pieces []Piece

	// PreCommit
	CommD  *cid.Cid
	CommR  *cid.Cid
	Proof  []byte
	Ticket api.SealTicket

	PreCommitMessage *cid.Cid

	// WaitSeed
	Seed api.SealSeed

	// Committing
	CommitMessage *cid.Cid

	// Faults
	FaultReportMsg *cid.Cid

	// Debug
	LastErr string

	Log []Log
}

func (t *SectorInfo) pieceInfos() []abi.PieceInfo {
	out := make([]abi.PieceInfo, len(t.Pieces))
	for i, piece := range t.Pieces {
		out[i] = abi.PieceInfo{
			Size:     piece.Size.Padded(),
			PieceCID: piece.CommP,
		}
	}
	return out
}

func (t *SectorInfo) deals() []abi.DealID {
	out := make([]abi.DealID, 0, len(t.Pieces))
	for _, piece := range t.Pieces {
		if piece.DealID == nil {
			continue
		}
		out = append(out, *piece.DealID)
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
