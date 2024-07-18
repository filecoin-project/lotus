package storiface

import (
	"context"
	"io"
	"net/http"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"
)

type Data = io.Reader

// Reader is a fully-featured Reader. It is the
// union of the standard IO sequential access method (Read), with seeking
// ability (Seek), as well random access (ReadAt).
type Reader interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
}

type SectorRef struct {
	ID        abi.SectorID
	ProofType abi.RegisteredSealProof
}

var NoSectorRef = SectorRef{}

// PieceNumber is a reference to a piece in the storage system; mapping between
// pieces in the storage system and piece CIDs is maintained by the storage index
type PieceNumber uint64

func (pn PieceNumber) Ref() SectorRef {
	return SectorRef{
		ID:        abi.SectorID{Miner: 0, Number: abi.SectorNumber(pn)},
		ProofType: abi.RegisteredSealProof_StackedDrg64GiBV1, // This only cares about TreeD which is the same for all sizes
	}
}

type ProverPoSt interface {
	GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error)
	GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, ppt abi.RegisteredPoStProof, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) (proof []proof.PoStProof, skipped []abi.SectorID, err error)

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

	FinalizeSector(ctx context.Context, sector SectorRef) error

	// ReleaseUnsealed marks parts of the unsealed sector file as safe to drop
	//  (called by the fsm on restart, allows storage to keep no persistent
	//   state about unsealed fast-retrieval copies)
	ReleaseUnsealed(ctx context.Context, sector SectorRef, keepUnsealed []Range) error
	// ReleaseSectorKey removes `sealed` from all storage
	// called after successful sector upgrade
	ReleaseSectorKey(ctx context.Context, sector SectorRef) error
	// ReleaseReplicaUpgrade removes `update` / `update-cache` from all storage
	// called when aborting sector upgrade
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

	FinalizeReplicaUpdate(ctx context.Context, sector SectorRef) error

	DownloadSectorData(ctx context.Context, sector SectorRef, finalized bool, src map[SectorFileType]SectorLocation) error
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

type SectorLocation struct {
	// Local when set to true indicates to lotus that sector data is already
	// available locally; When set lotus will skip fetching sector data, and
	// only check that sector data exists in sector storage
	Local bool

	// URL to the sector data
	// For sealed/unsealed sector, lotus expects octet-stream
	// For cache, lotus expects a tar archive with cache files
	// Valid schemas:
	// - http:// / https://
	URL string

	// optional http headers to use when requesting sector data
	Headers []SecDataHttpHeader
}

func (sd *SectorLocation) HttpHeaders() http.Header {
	out := http.Header{}
	for _, header := range sd.Headers {
		out[header.Key] = append(out[header.Key], header.Value)
	}
	return out
}

// note: we can't use http.Header as that's backed by a go map, which is all kinds of messy

type SecDataHttpHeader struct {
	Key   string
	Value string
}

// StorageConfig .lotusstorage/storage.json
type StorageConfig struct {
	StoragePaths []LocalPath
}

type LocalPath struct {
	Path string
}

// LocalStorageMeta [path]/sectorstore.json
type LocalStorageMeta struct {
	ID ID

	// A high weight means data is more likely to be stored in this path
	Weight uint64 // 0 = readonly

	// Intermediate data for the sealing process will be stored here
	CanSeal bool

	// Finalized sectors that will be proved over time will be stored here
	CanStore bool

	// MaxStorage specifies the maximum number of bytes to use for sector storage
	// (0 = unlimited)
	MaxStorage uint64

	// List of storage groups this path belongs to
	Groups []string

	// List of storage groups to which data from this path can be moved. If none
	// are specified, allow to all
	AllowTo []string

	// AllowTypes lists sector file types which are allowed to be put into this
	// path. If empty, all file types are allowed.
	//
	// Valid values:
	// - "unsealed"
	// - "sealed"
	// - "cache"
	// - "update"
	// - "update-cache"
	// Any other value will generate a warning and be ignored.
	AllowTypes []string

	// DenyTypes lists sector file types which aren't allowed to be put into this
	// path.
	//
	// Valid values:
	// - "unsealed"
	// - "sealed"
	// - "cache"
	// - "update"
	// - "update-cache"
	// Any other value will generate a warning and be ignored.
	DenyTypes []string

	// AllowMiners lists miner IDs which are allowed to store their sector data into
	// this path. If empty, all miner IDs are allowed
	AllowMiners []string

	// DenyMiners lists miner IDs which are denied to store their sector data into
	// this path
	DenyMiners []string
}
