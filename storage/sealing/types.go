package sealing

import (
	"bytes"
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-storage/storage"
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
	State    SectorState
	SectorID abi.SectorNumber // TODO: this field's name should be changed to SectorNumber
	Nonce    uint64           // TODO: remove

	SectorType abi.RegisteredProof

	// Packing

	Pieces []Piece

	// PreCommit1
	TicketValue   abi.SealRandomness
	TicketEpoch   abi.ChainEpoch
	PreCommit1Out storage.PreCommit1Out

	// PreCommit2
	CommD *cid.Cid
	CommR *cid.Cid
	Proof []byte

	PreCommitMessage *cid.Cid

	// WaitSeed
	SeedValue abi.InteractiveSealRandomness
	SeedEpoch abi.ChainEpoch

	// Committing
	CommitMessage *cid.Cid
	InvalidProofs uint64 // failed proof computations (doesn't validate with proof inputs)

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

type TicketFn func(context.Context) (abi.SealRandomness, abi.ChainEpoch, error)

type SectorIDCounter interface {
	Next() (abi.SectorNumber, error)
}

type TipSetToken []byte

type MsgLookup struct {
	Receipt   MessageReceipt
	TipSetTok TipSetToken
	Height    abi.ChainEpoch
}

type MessageReceipt struct {
	ExitCode exitcode.ExitCode
	Return   []byte
	GasUsed  int64
}

func (mr *MessageReceipt) Equals(o *MessageReceipt) bool {
	return mr.ExitCode == o.ExitCode && bytes.Equal(mr.Return, o.Return) && mr.GasUsed == o.GasUsed
}

type MarketDeal struct {
	Proposal market.DealProposal
	State    market.DealState
}
