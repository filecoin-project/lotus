package storiface

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
)

type WorkerID uuid.UUID // worker session UUID

func (w WorkerID) String() string {
	return uuid.UUID(w).String()
}

type WorkerInfo struct {
	Hostname string

	// IgnoreResources indicates whether the worker's available resources should
	// be used ignored (true) or used (false) for the purposes of scheduling and
	// task assignment. Only supported on local workers. Used for testing.
	// Default should be false (zero value, i.e. resources taken into account).
	IgnoreResources bool
	Resources       WorkerResources
}

type WorkerResources struct {
	MemPhysical uint64
	MemUsed     uint64
	MemSwap     uint64
	MemSwapUsed uint64

	CPUs uint64 // Logical cores
	GPUs []string

	// if nil use the default resource table
	Resources map[sealtasks.TaskType]map[abi.RegisteredSealProof]Resources
}

func (wr WorkerResources) ResourceSpec(spt abi.RegisteredSealProof, tt sealtasks.TaskType) Resources {
	res := ResourceTable[tt][spt]

	// if the worker specifies custom resource table, prefer that
	if wr.Resources != nil {
		tr, ok := wr.Resources[tt]
		if !ok {
			return res
		}

		r, ok := tr[spt]
		if ok {
			return r
		}
	}

	// otherwise, use the default resource table
	return res
}

// PrepResourceSpec is like ResourceSpec, but meant for use limiting parallel preparing
// tasks.
func (wr WorkerResources) PrepResourceSpec(spt abi.RegisteredSealProof, tt, prepTT sealtasks.TaskType) Resources {
	res := wr.ResourceSpec(spt, tt)

	if prepTT != tt && prepTT != sealtasks.TTNoop {
		prepRes := wr.ResourceSpec(spt, prepTT)
		res.MaxConcurrent = prepRes.MaxConcurrent
	}

	// otherwise, use the default resource table
	return res
}

type WorkerStats struct {
	Info    WorkerInfo
	Tasks   []sealtasks.TaskType
	Enabled bool

	MemUsedMin uint64
	MemUsedMax uint64
	GpuUsed    float64 // nolint
	CpuUse     uint64  // nolint

	TaskCounts map[string]int
}

const (
	RWPrepared = 1
	RWRunning  = 0
	RWRetWait  = -1
	RWReturned = -2
	RWRetDone  = -3
)

type WorkerJob struct {
	ID     CallID
	Sector abi.SectorID
	Task   sealtasks.TaskType

	// 2+ - assigned
	// 1  - prepared
	// 0  - running
	// -1 - ret-wait
	// -2 - returned
	// -3 - ret-done
	RunWait int
	Start   time.Time

	Hostname string `json:",omitempty"` // optional, set for ret-wait jobs
}

type CallID struct {
	Sector abi.SectorID
	ID     uuid.UUID
}

func (c CallID) String() string {
	return fmt.Sprintf("%d-%d-%s", c.Sector.Miner, c.Sector.Number, c.ID)
}

var _ fmt.Stringer = &CallID{}

var UndefCall CallID

type WorkerCalls interface {
	// async
	DataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData Data) (CallID, error)
	AddPiece(ctx context.Context, sector SectorRef, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData Data) (CallID, error)
	SealPreCommit1(ctx context.Context, sector SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (CallID, error)
	SealPreCommit2(ctx context.Context, sector SectorRef, pc1o PreCommit1Out) (CallID, error)
	SealCommit1(ctx context.Context, sector SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids SectorCids) (CallID, error)
	SealCommit2(ctx context.Context, sector SectorRef, c1o Commit1Out) (CallID, error)
	FinalizeSector(ctx context.Context, sector SectorRef) (CallID, error)
	FinalizeReplicaUpdate(ctx context.Context, sector SectorRef) (CallID, error)
	ReleaseUnsealed(ctx context.Context, sector SectorRef, safeToFree []Range) (CallID, error)
	ReplicaUpdate(ctx context.Context, sector SectorRef, pieces []abi.PieceInfo) (CallID, error)
	ProveReplicaUpdate1(ctx context.Context, sector SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (CallID, error)
	ProveReplicaUpdate2(ctx context.Context, sector SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs ReplicaVanillaProofs) (CallID, error)
	GenerateSectorKeyFromData(ctx context.Context, sector SectorRef, commD cid.Cid) (CallID, error)
	MoveStorage(ctx context.Context, sector SectorRef, types SectorFileType) (CallID, error)
	UnsealPiece(context.Context, SectorRef, UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (CallID, error)
	Fetch(context.Context, SectorRef, SectorFileType, PathType, AcquireMode) (CallID, error)
	DownloadSectorData(ctx context.Context, sector SectorRef, finalized bool, src map[SectorFileType]SectorLocation) (CallID, error)

	// sync
	GenerateWinningPoSt(ctx context.Context, ppt abi.RegisteredPoStProof, mid abi.ActorID, sectors []PostSectorChallenge, randomness abi.PoStRandomness) ([]proof.PoStProof, error)
	GenerateWindowPoSt(ctx context.Context, ppt abi.RegisteredPoStProof, mid abi.ActorID, sectors []PostSectorChallenge, partitionIdx int, randomness abi.PoStRandomness) (WindowPoStResult, error)
}

type WindowPoStResult struct {
	PoStProofs proof.PoStProof
	Skipped    []abi.SectorID
}

type PostSectorChallenge struct {
	SealProof    abi.RegisteredSealProof
	SectorNumber abi.SectorNumber
	SealedCID    cid.Cid
	Challenge    []uint64
	Update       bool
}

type FallbackChallenges struct {
	Sectors    []abi.SectorNumber
	Challenges map[abi.SectorNumber][]uint64
}

type ErrorCode int

const (
	ErrUnknown ErrorCode = iota
)

const (
	// Temp Errors
	ErrTempUnknown ErrorCode = iota + 100
	ErrTempWorkerRestart
	ErrTempAllocateSpace
)

type WorkError interface {
	ErrCode() ErrorCode
}

type CallError struct {
	Code    ErrorCode
	Message string
	sub     error
}

func (c *CallError) ErrCode() ErrorCode {
	return c.Code
}

func (c *CallError) Error() string {
	return fmt.Sprintf("storage call error %d: %s", c.Code, c.Message)
}

func (c *CallError) Unwrap() error {
	if c.sub != nil {
		return c.sub
	}

	return errors.New(c.Message)
}

var _ WorkError = &CallError{}

func Err(code ErrorCode, sub error) *CallError {
	return &CallError{
		Code:    code,
		Message: sub.Error(),

		sub: sub,
	}
}

type WorkerReturn interface {
	ReturnDataCid(ctx context.Context, callID CallID, pi abi.PieceInfo, err *CallError) error
	ReturnAddPiece(ctx context.Context, callID CallID, pi abi.PieceInfo, err *CallError) error
	ReturnSealPreCommit1(ctx context.Context, callID CallID, p1o PreCommit1Out, err *CallError) error
	ReturnSealPreCommit2(ctx context.Context, callID CallID, sealed SectorCids, err *CallError) error
	ReturnSealCommit1(ctx context.Context, callID CallID, out Commit1Out, err *CallError) error
	ReturnSealCommit2(ctx context.Context, callID CallID, proof Proof, err *CallError) error
	ReturnFinalizeSector(ctx context.Context, callID CallID, err *CallError) error
	ReturnReleaseUnsealed(ctx context.Context, callID CallID, err *CallError) error
	ReturnReplicaUpdate(ctx context.Context, callID CallID, out ReplicaUpdateOut, err *CallError) error
	ReturnProveReplicaUpdate1(ctx context.Context, callID CallID, proofs ReplicaVanillaProofs, err *CallError) error
	ReturnProveReplicaUpdate2(ctx context.Context, callID CallID, proof ReplicaUpdateProof, err *CallError) error
	ReturnGenerateSectorKeyFromData(ctx context.Context, callID CallID, err *CallError) error
	ReturnFinalizeReplicaUpdate(ctx context.Context, callID CallID, err *CallError) error
	ReturnMoveStorage(ctx context.Context, callID CallID, err *CallError) error
	ReturnUnsealPiece(ctx context.Context, callID CallID, err *CallError) error
	ReturnReadPiece(ctx context.Context, callID CallID, ok bool, err *CallError) error
	ReturnDownloadSector(ctx context.Context, callID CallID, err *CallError) error
	ReturnFetch(ctx context.Context, callID CallID, err *CallError) error
}
