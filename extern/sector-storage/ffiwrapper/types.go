package ffiwrapper

import (
	"context"
	"io"

	"github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

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
}

type Verifier interface {
	VerifySeal(proof.SealVerifyInfo) (bool, error)
	VerifyAggregateSeals(aggregate proof.AggregateSealVerifyProofAndInfos) (bool, error)
	VerifyReplicaUpdate(update proof.ReplicaUpdateInfo) (bool, error)
	VerifyWinningPoSt(ctx context.Context, info proof.WinningPoStVerifyInfo) (bool, error)
	VerifyWindowPoSt(ctx context.Context, info proof.WindowPoStVerifyInfo) (bool, error)

	GenerateWinningPoStSectorChallenge(context.Context, abi.RegisteredPoStProof, abi.ActorID, abi.PoStRandomness, uint64) ([]uint64, error)
}

// Prover contains cheap proving-related methods
type Prover interface {
	// TODO: move GenerateWinningPoStSectorChallenge from the Verifier interface to here

	AggregateSealProofs(aggregateInfo proof.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error)
}

type SectorProvider interface {
	// * returns storiface.ErrSectorNotFound if a requested existing sector doesn't exist
	// * returns an error when allocate is set, and existing isn't, and the sector exists
	AcquireSector(ctx context.Context, id storage.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, ptype storiface.PathType) (storiface.SectorPaths, func(), error)
}

var _ SectorProvider = &basicfs.Provider{}
