package sealing

import (
	"bytes"
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-storage/storage"
)

// PieceWithOptionalDealInfo is a tuple of piece and deal info
type PieceWithDealInfo struct {
	Piece    abi.PieceInfo
	DealInfo DealInfo
}

// PieceWithOptionalDealInfo is a tuple of piece info and optional deal
type PieceWithOptionalDealInfo struct {
	Piece    abi.PieceInfo
	DealInfo *DealInfo // nil for pieces which do not yet appear in self-deals
}

// DealInfo is a tuple of deal identity and its schedule
type DealInfo struct {
	DealID       abi.DealID
	DealSchedule DealSchedule
}

// DealSchedule communicates the time interval of a storage deal. The deal must
// appear in a sealed (proven) sector no later than StartEpoch, otherwise it
// is invalid.
type DealSchedule struct {
	StartEpoch abi.ChainEpoch
	EndEpoch   abi.ChainEpoch
}

type Log struct {
	Timestamp uint64
	Trace     string // for errors

	Message string

	// additional data (Event info)
	Kind string
}

type SectorInfo struct {
	State        SectorState
	SectorNumber abi.SectorNumber // TODO: this field's name should be changed to SectorNumber
	Nonce        uint64           // TODO: remove

	SectorType abi.RegisteredProof

	// Packing
	PiecesWithOptionalDealInfo []PieceWithOptionalDealInfo

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
	out := make([]abi.PieceInfo, len(t.PiecesWithOptionalDealInfo))
	for i, pdi := range t.PiecesWithOptionalDealInfo {
		out[i] = abi.PieceInfo{
			Size:     pdi.Piece.Size,
			PieceCID: pdi.Piece.PieceCID,
		}
	}
	return out
}

func (t *SectorInfo) dealIDs() []abi.DealID {
	out := make([]abi.DealID, 0, len(t.PiecesWithOptionalDealInfo))
	for _, pdi := range t.PiecesWithOptionalDealInfo {
		if pdi.DealInfo == nil {
			continue
		}
		out = append(out, pdi.DealInfo.DealID)
	}
	return out
}

func (t *SectorInfo) existingPieceSizes() []abi.UnpaddedPieceSize {
	out := make([]abi.UnpaddedPieceSize, len(t.PiecesWithOptionalDealInfo))
	for i, pdi := range t.PiecesWithOptionalDealInfo {
		out[i] = pdi.Piece.Size.Unpadded()
	}
	return out
}

type TicketFn func(ctx context.Context, tok TipSetToken) (abi.SealRandomness, abi.ChainEpoch, error)

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
