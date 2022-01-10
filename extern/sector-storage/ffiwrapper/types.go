package ffiwrapper

import (
	"context"
	"io"

	proof7 "github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/specs-actors/actors/runtime/proof"
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

type Validator interface {
	CanCommit(sector storiface.SectorPaths) (bool, error)
	CanProve(sector storiface.SectorPaths) (bool, error)
}

type StorageSealer interface {
	storage.Sealer
	storage.Storage
}

type Storage interface {
	storage.Prover
	StorageSealer

	UnsealPiece(ctx context.Context, sector storage.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd cid.Cid) error
	ReadPiece(ctx context.Context, writer io.Writer, sector storage.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error)

	WorkerProver
}

type Verifier interface {
	VerifySeal(proof5.SealVerifyInfo) (bool, error)
	VerifyAggregateSeals(aggregate proof5.AggregateSealVerifyProofAndInfos) (bool, error)
	VerifyReplicaUpdate(update proof7.ReplicaUpdateInfo) (bool, error)
	VerifyWinningPoSt(ctx context.Context, info proof5.WinningPoStVerifyInfo) (bool, error)
	VerifyWindowPoSt(ctx context.Context, info proof5.WindowPoStVerifyInfo) (bool, error)

	GenerateWinningPoStSectorChallenge(context.Context, abi.RegisteredPoStProof, abi.ActorID, abi.PoStRandomness, uint64) ([]uint64, error)
}

// Prover contains cheap proving-related methods
type Prover interface {
	// TODO: move GenerateWinningPoStSectorChallenge from the Verifier interface to here

	AggregateSealProofs(aggregateInfo proof5.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error)
}

type SectorProvider interface {
	// * returns storiface.ErrSectorNotFound if a requested existing sector doesn't exist
	// * returns an error when allocate is set, and existing isn't, and the sector exists
	AcquireSector(ctx context.Context, id storage.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, ptype storiface.PathType) (storiface.SectorPaths, func(), error)
	AcquireSectorPaths(ctx context.Context, id storage.SectorRef, existing storiface.SectorFileType, ptype storiface.PathType) (storiface.SectorPaths, func(), error)
}

var _ SectorProvider = &basicfs.Provider{}

type MinerProver interface {
	PubSectorToPriv(ctx context.Context, mid abi.ActorID, sectorInfo []proof5.SectorInfo, faults []abi.SectorNumber, rpt func(abi.RegisteredSealProof) (abi.RegisteredPoStProof, error)) (SortedPrivateSectorInfo, []abi.SectorID, func(), error)
	SplitSortedPrivateSectorInfo(ctx context.Context, privsector SortedPrivateSectorInfo, offset int, end int) (SortedPrivateSectorInfo, error)
	GeneratePoStFallbackSectorChallenges(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, sectorIds []abi.SectorNumber) (*FallbackChallenges, error)
}

type WindowPoStResult struct {
	PoStProofs proof.PoStProof
	Skipped    []abi.SectorID
}

// type SortedPrivateSectorInfo ffi.SortedPrivateSectorInfo
type FallbackChallenges struct {
	Fc ffi.FallbackChallenges
}

type SortedPrivateSectorInfo struct {
	Spsi ffi.SortedPrivateSectorInfo
}

type PrivateSectorInfo struct {
	Psi ffi.PrivateSectorInfo
}

type WorkerProver interface {
	GenerateWindowPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte, partitionIdx int) (*proof.PoStProof, error)
	GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, vanillas [][]byte) ([]proof.PoStProof, error)
}

type WorkerCalls interface {
	GenerateWindowPoSt(ctx context.Context, mid abi.ActorID, privsectors SortedPrivateSectorInfo, partitionIdx int, offset int, randomness abi.PoStRandomness, sectorChallenges *FallbackChallenges) (WindowPoStResult, error)
	GenerateWinningPoSt(ctx context.Context, mid abi.ActorID, privsectors SortedPrivateSectorInfo, randomness abi.PoStRandomness, sectorChallenges *FallbackChallenges) ([]proof.PoStProof, error)
}
