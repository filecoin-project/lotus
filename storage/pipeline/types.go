package sealing

import (
	"context"
	"encoding/json"
	"io"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

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

type UniversalPieceInfo interface {
	Impl() piece.PieceDealInfo
	String() string
	Key() piece.PieceKey

	Valid(nv network.Version) error
	StartEpoch() (abi.ChainEpoch, error)
	EndEpoch() (abi.ChainEpoch, error)
	PieceCID() cid.Cid
	KeepUnsealedRequested() bool

	GetAllocation(ctx context.Context, aapi piece.AllocationAPI, tsk types.TipSetKey) (*verifreg.Allocation, error)
}

type SectorInfo struct {
	State        SectorState
	SectorNumber abi.SectorNumber

	SectorType abi.RegisteredSealProof

	// Packing
	CreationTime int64 // unix seconds
	Pieces       []SafeSectorPiece

	// PreCommit1
	TicketValue   abi.SealRandomness
	TicketEpoch   abi.ChainEpoch
	PreCommit1Out storiface.PreCommit1Out

	PreCommit1Fails uint64

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
	CCPieces             []SafeSectorPiece
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
		out[i] = p.Piece()
	}
	return out
}

func (t *SectorInfo) nonPaddingPieceInfos() []abi.PieceInfo {
	out := make([]abi.PieceInfo, len(t.Pieces))
	for i, p := range t.Pieces {
		if !p.HasDealInfo() {
			continue
		}

		out[i] = p.Piece()
	}
	return out
}

func (t *SectorInfo) existingPieceSizes() []abi.UnpaddedPieceSize {
	out := make([]abi.UnpaddedPieceSize, len(t.Pieces))
	for i, p := range t.Pieces {
		out[i] = p.Piece().Size.Unpadded()
	}
	return out
}

func (t *SectorInfo) hasData() bool {
	for _, piece := range t.Pieces {
		if piece.HasDealInfo() {
			return true
		}
	}

	return false
}

func (t *SectorInfo) sealingCtx(ctx context.Context) context.Context {
	// TODO: can also take start epoch into account to give priority to sectors
	//  we need sealed sooner

	if t.hasData() {
		return sealer.WithPriority(ctx, DealSectorPriority)
	}

	return ctx
}

// Returns list of offset/length tuples of sector data ranges which clients
// requested to keep unsealed
func (t *SectorInfo) keepUnsealedRanges(pieces []SafeSectorPiece, invert, alwaysKeep bool) []storiface.Range {
	var out []storiface.Range

	var at abi.UnpaddedPieceSize
	for _, piece := range pieces {
		psize := piece.Piece().Size.Unpadded()
		at += psize

		if !piece.HasDealInfo() {
			continue
		}

		keep := piece.DealInfo().KeepUnsealedRequested() || alwaysKeep

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

// SafeSectorPiece is a wrapper around SectorPiece which makes it hard to misuse
// especially by making it hard to access raw Deal / DDO info
type SafeSectorPiece struct {
	real api.SectorPiece
}

func SafePiece(piece api.SectorPiece) SafeSectorPiece {
	return SafeSectorPiece{piece}
}

var _ UniversalPieceInfo = &SafeSectorPiece{}

func (sp *SafeSectorPiece) Piece() abi.PieceInfo {
	return sp.real.Piece
}

func (sp *SafeSectorPiece) HasDealInfo() bool {
	return sp.real.DealInfo != nil
}

func (sp *SafeSectorPiece) DealInfo() UniversalPieceInfo {
	return sp.real.DealInfo
}

// UnmarshalCBOR is a cbor passthrough
func (sp *SafeSectorPiece) UnmarshalCBOR(r io.Reader) (err error) {
	return sp.real.UnmarshalCBOR(r)
}

func (sp *SafeSectorPiece) MarshalCBOR(w io.Writer) error {
	return sp.real.MarshalCBOR(w)
}

// UnmarshalJSON is a json passthrough
func (sp *SafeSectorPiece) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &sp.real)
}

func (sp *SafeSectorPiece) MarshalJSON() ([]byte, error) {
	return json.Marshal(sp.real)
}

type handleDealInfoParams struct {
	FillerHandler        func(UniversalPieceInfo) error
	BuiltinMarketHandler func(UniversalPieceInfo) error
	DDOHandler           func(UniversalPieceInfo) error
}

func (sp *SafeSectorPiece) handleDealInfo(params handleDealInfoParams) error {
	if !sp.HasDealInfo() {
		if params.FillerHandler == nil {
			return xerrors.Errorf("FillerHandler is not provided")
		}
		return params.FillerHandler(sp)
	}

	if sp.real.DealInfo.PublishCid != nil {
		if params.BuiltinMarketHandler == nil {
			return xerrors.Errorf("BuiltinMarketHandler is not provided")
		}
		return params.BuiltinMarketHandler(sp)
	}

	if params.DDOHandler == nil {
		return xerrors.Errorf("DDOHandler is not provided")
	}
	return params.DDOHandler(sp)
}

// SectorPiece Proxy

func (sp *SafeSectorPiece) Impl() piece.PieceDealInfo {
	if !sp.HasDealInfo() {
		return piece.PieceDealInfo{}
	}

	return sp.real.DealInfo.Impl()
}

func (sp *SafeSectorPiece) String() string {
	if !sp.HasDealInfo() {
		return "<no deal info>"
	}

	return sp.real.DealInfo.String()
}

func (sp *SafeSectorPiece) Key() piece.PieceKey {
	return sp.real.DealInfo.Key()
}

func (sp *SafeSectorPiece) Valid(nv network.Version) error {
	return sp.real.DealInfo.Valid(nv)
}

func (sp *SafeSectorPiece) StartEpoch() (abi.ChainEpoch, error) {
	if !sp.HasDealInfo() {
		return 0, xerrors.Errorf("no deal info")
	}

	return sp.real.DealInfo.StartEpoch()
}

func (sp *SafeSectorPiece) EndEpoch() (abi.ChainEpoch, error) {
	if !sp.HasDealInfo() {
		return 0, xerrors.Errorf("no deal info")
	}

	return sp.real.DealInfo.EndEpoch()
}

func (sp *SafeSectorPiece) PieceCID() cid.Cid {
	if !sp.HasDealInfo() {
		return sp.real.Piece.PieceCID
	}

	return sp.real.DealInfo.PieceCID()
}

func (sp *SafeSectorPiece) KeepUnsealedRequested() bool {
	if !sp.HasDealInfo() {
		return false
	}

	return sp.real.DealInfo.KeepUnsealedRequested()
}

func (sp *SafeSectorPiece) GetAllocation(ctx context.Context, aapi piece.AllocationAPI, tsk types.TipSetKey) (*verifreg.Allocation, error) {
	if !sp.HasDealInfo() {
		return nil, xerrors.Errorf("no deal info")
	}

	return sp.real.DealInfo.GetAllocation(ctx, aapi, tsk)
}
