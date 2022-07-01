package storiface

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"
)

type Data = io.Reader

type SectorRef struct {
	ID        abi.SectorID
	ProofType abi.RegisteredSealProof
}

var NoSectorRef = SectorRef{}

type ProverPoSt interface {
	GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error)
	GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) (proof []proof.PoStProof, skipped []abi.SectorID, err error)

	GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]proof.PoStProof, error)
	GenerateWindowPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte, partitionIdx int) (proof.PoStProof, error)
}

type PreCommit1Out []byte

type Commit1Out []byte

type Proof []byte

type SectorCids struct {
	Unsealed cid.Cid
	Sealed   cid.Cid
}

type Range struct {
	Offset abi.UnpaddedPieceSize
	Size   abi.UnpaddedPieceSize
}

type ReplicaUpdateProof []byte
type ReplicaVanillaProofs [][]byte

type ReplicaUpdateOut struct {
	NewSealed   cid.Cid
	NewUnsealed cid.Cid
}

type Sealer interface {
	NewSector(ctx context.Context, sector SectorRef) error
	DataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData Data) (abi.PieceInfo, error)
	AddPiece(ctx context.Context, sector SectorRef, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData Data) (abi.PieceInfo, error)

	SealPreCommit1(ctx context.Context, sector SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (PreCommit1Out, error)
	SealPreCommit2(ctx context.Context, sector SectorRef, pc1o PreCommit1Out) (SectorCids, error)

	SealCommit1(ctx context.Context, sector SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids SectorCids) (Commit1Out, error)
	SealCommit2(ctx context.Context, sector SectorRef, c1o Commit1Out) (Proof, error)

	FinalizeSector(ctx context.Context, sector SectorRef, keepUnsealed []Range) error

	// ReleaseUnsealed marks parts of the unsealed sector file as safe to drop
	//  (called by the fsm on restart, allows storage to keep no persistent
	//   state about unsealed fast-retrieval copies)
	ReleaseUnsealed(ctx context.Context, sector SectorRef, safeToFree []Range) error
	ReleaseSectorKey(ctx context.Context, sector SectorRef) error
	ReleaseReplicaUpgrade(ctx context.Context, sector SectorRef) error

	// Removes all data associated with the specified sector
	Remove(ctx context.Context, sector SectorRef) error

	// Generate snap deals replica update
	ReplicaUpdate(ctx context.Context, sector SectorRef, pieces []abi.PieceInfo) (ReplicaUpdateOut, error)

	// Prove that snap deals replica was done correctly
	ProveReplicaUpdate1(ctx context.Context, sector SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (ReplicaVanillaProofs, error)
	ProveReplicaUpdate2(ctx context.Context, sector SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs ReplicaVanillaProofs) (ReplicaUpdateProof, error)

	// GenerateSectorKeyFromData computes sector key given unsealed data and updated replica
	GenerateSectorKeyFromData(ctx context.Context, sector SectorRef, unsealed cid.Cid) error

	FinalizeReplicaUpdate(ctx context.Context, sector SectorRef, keepUnsealed []Range) error
}

type Unsealer interface {
	UnsealPiece(ctx context.Context, sector SectorRef, offset UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd cid.Cid) error
	ReadPiece(ctx context.Context, writer io.Writer, sector SectorRef, offset UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error)
}

type Storage interface {
	ProverPoSt
	Sealer
	Unsealer
}

type Validator interface {
	CanCommit(sector SectorPaths) (bool, error)
	CanProve(sector SectorPaths) (bool, error)
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
