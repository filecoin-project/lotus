package sealing

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

//go:generate go run github.com/golang/mock/mockgen -destination=mocks/statemachine.go -package=mocks . Context

// Context is a go-statemachine context
type Context interface {
	Context() context.Context
	Send(evt interface{}) error
}

type Log struct {
	Timestamp uint64
	Trace     string // for errors

	Message string

	// additional data (Event info)
	Kind string
}

type ReturnState string

const (
	RetPreCommit1      = ReturnState(PreCommit1)
	RetPreCommitting   = ReturnState(PreCommitting)
	RetPreCommitFailed = ReturnState(PreCommitFailed)
	RetCommitFailed    = ReturnState(CommitFailed)
)

type SectorInfo struct {
	State        SectorState
	SectorNumber abi.SectorNumber

	SectorType abi.RegisteredSealProof

	// Packing
	CreationTime int64 // unix seconds
	Pieces       []api.SectorPiece

	// PreCommit1
	TicketValue   abi.SealRandomness
	TicketEpoch   abi.ChainEpoch
	PreCommit1Out storiface.PreCommit1Out

	// PreCommit2
	CommD *cid.Cid
	CommR *cid.Cid // SectorKey
	Proof []byte

	PreCommitDeposit big.Int
	PreCommitMessage *cid.Cid
	PreCommitTipSet  types.TipSetKey

	PreCommit2Fails uint64

	// WaitSeed
	SeedValue abi.InteractiveSealRandomness
	SeedEpoch abi.ChainEpoch

	// Committing
	CommitMessage *cid.Cid
	InvalidProofs uint64 // failed proof computations (doesn't validate with proof inputs; can't compute)

	// CCUpdate
	CCUpdate             bool
	CCPieces             []api.SectorPiece
	UpdateSealed         *cid.Cid
	UpdateUnsealed       *cid.Cid
	ReplicaUpdateProof   storiface.ReplicaUpdateProof
	ReplicaUpdateMessage *cid.Cid

	// Faults
	FaultReportMsg *cid.Cid

	// Recovery / Import
	Return ReturnState

	// Termination
	TerminateMessage *cid.Cid
	TerminatedAt     abi.ChainEpoch

	// Remote import
	RemoteDataUnsealed        *storiface.SectorLocation
	RemoteDataSealed          *storiface.SectorLocation
	RemoteDataCache           *storiface.SectorLocation
	RemoteCommit1Endpoint     string
	RemoteCommit2Endpoint     string
	RemoteSealingDoneEndpoint string
	RemoteDataFinalized       bool

	// Debug
	LastErr string

	Log []Log
}

func (t *SectorInfo) pieceInfos() []abi.PieceInfo {
	out := make([]abi.PieceInfo, len(t.Pieces))
	for i, p := range t.Pieces {
		out[i] = p.Piece
	}
	return out
}

func (t *SectorInfo) dealIDs() []abi.DealID {
	out := make([]abi.DealID, 0, len(t.Pieces))
	for _, p := range t.Pieces {
		if p.DealInfo == nil {
			continue
		}
		out = append(out, p.DealInfo.DealID)
	}
	return out
}

func (t *SectorInfo) existingPieceSizes() []abi.UnpaddedPieceSize {
	out := make([]abi.UnpaddedPieceSize, len(t.Pieces))
	for i, p := range t.Pieces {
		out[i] = p.Piece.Size.Unpadded()
	}
	return out
}

func (t *SectorInfo) hasDeals() bool {
	for _, piece := range t.Pieces {
		if piece.DealInfo != nil {
			return true
		}
	}

	return false
}

func (t *SectorInfo) sealingCtx(ctx context.Context) context.Context {
	// TODO: can also take start epoch into account to give priority to sectors
	//  we need sealed sooner

	if t.hasDeals() {
		return sealer.WithPriority(ctx, DealSectorPriority)
	}

	return ctx
}

// Returns list of offset/length tuples of sector data ranges which clients
// requested to keep unsealed
func (t *SectorInfo) keepUnsealedRanges(pieces []api.SectorPiece, invert, alwaysKeep bool) []storiface.Range {
	var out []storiface.Range

	var at abi.UnpaddedPieceSize
	for _, piece := range pieces {
		psize := piece.Piece.Size.Unpadded()
		at += psize

		if piece.DealInfo == nil {
			continue
		}

		keep := piece.DealInfo.KeepUnsealed || alwaysKeep

		if keep == invert {
			continue
		}

		out = append(out, storiface.Range{
			Offset: at - psize,
			Size:   psize,
		})
	}

	return out
}

// SealingStateEvt is a journal event that records a sector state transition.
type SealingStateEvt struct {
	SectorNumber abi.SectorNumber
	SectorType   abi.RegisteredSealProof
	From         SectorState
	After        SectorState
	Error        string
}
