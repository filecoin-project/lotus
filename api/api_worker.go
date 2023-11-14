package api

import (
	"context"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

//                       MODIFYING THE API INTERFACE
//
// When adding / changing methods in this file:
// * Do the change here
// * Adjust implementation in `node/impl/`
// * Run `make gen` - this will:
//  * Generate proxy structs
//  * Generate mocks
//  * Generate markdown docs
//  * Generate openrpc blobs

type Worker interface {
	Version(context.Context) (Version, error) //perm:admin

	// TaskType -> Weight
	TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error) //perm:admin
	Paths(context.Context) ([]storiface.StoragePath, error)             //perm:admin
	Info(context.Context) (storiface.WorkerInfo, error)                 //perm:admin

	// storiface.WorkerCalls
	DataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (storiface.CallID, error)                                                                                        //perm:admin
	AddPiece(ctx context.Context, sector storiface.SectorRef, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (storiface.CallID, error)                    //perm:admin
	SealPreCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storiface.CallID, error)                                                             //perm:admin
	SealPreCommit2(ctx context.Context, sector storiface.SectorRef, pc1o storiface.PreCommit1Out) (storiface.CallID, error)                                                                                  //perm:admin
	SealCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storiface.SectorCids) (storiface.CallID, error) //perm:admin
	SealCommit2(ctx context.Context, sector storiface.SectorRef, c1o storiface.Commit1Out) (storiface.CallID, error)                                                                                         //perm:admin
	FinalizeSector(ctx context.Context, sector storiface.SectorRef) (storiface.CallID, error)                                                                                                                //perm:admin
	FinalizeReplicaUpdate(ctx context.Context, sector storiface.SectorRef) (storiface.CallID, error)                                                                                                         //perm:admin
	ReplicaUpdate(ctx context.Context, sector storiface.SectorRef, pieces []abi.PieceInfo) (storiface.CallID, error)                                                                                         //perm:admin
	ProveReplicaUpdate1(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (storiface.CallID, error)                                                                //perm:admin
	ProveReplicaUpdate2(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs storiface.ReplicaVanillaProofs) (storiface.CallID, error)                  //perm:admin
	GenerateSectorKeyFromData(ctx context.Context, sector storiface.SectorRef, commD cid.Cid) (storiface.CallID, error)                                                                                      //perm:admin
	ReleaseUnsealed(ctx context.Context, sector storiface.SectorRef, keepUnsealed []storiface.Range) (storiface.CallID, error)                                                                               //perm:admin
	MoveStorage(ctx context.Context, sector storiface.SectorRef, types storiface.SectorFileType) (storiface.CallID, error)                                                                                   //perm:admin
	UnsealPiece(context.Context, storiface.SectorRef, storiface.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (storiface.CallID, error)                                             //perm:admin
	Fetch(context.Context, storiface.SectorRef, storiface.SectorFileType, storiface.PathType, storiface.AcquireMode) (storiface.CallID, error)                                                               //perm:admin
	DownloadSectorData(ctx context.Context, sector storiface.SectorRef, finalized bool, src map[storiface.SectorFileType]storiface.SectorLocation) (storiface.CallID, error)                                 //perm:admin

	GenerateWinningPoSt(ctx context.Context, ppt abi.RegisteredPoStProof, mid abi.ActorID, sectors []storiface.PostSectorChallenge, randomness abi.PoStRandomness) ([]proof.PoStProof, error)                           //perm:admin
	GenerateWindowPoSt(ctx context.Context, ppt abi.RegisteredPoStProof, mid abi.ActorID, sectors []storiface.PostSectorChallenge, partitionIdx int, randomness abi.PoStRandomness) (storiface.WindowPoStResult, error) //perm:admin

	TaskDisable(ctx context.Context, tt sealtasks.TaskType) error //perm:admin
	TaskEnable(ctx context.Context, tt sealtasks.TaskType) error  //perm:admin

	// Storage / Other
	Remove(ctx context.Context, sector abi.SectorID) error //perm:admin

	StorageLocal(ctx context.Context) (map[storiface.ID]string, error)                   //perm:admin
	StorageAddLocal(ctx context.Context, path string) error                              //perm:admin
	StorageDetachLocal(ctx context.Context, path string) error                           //perm:admin
	StorageDetachAll(ctx context.Context) error                                          //perm:admin
	StorageRedeclareLocal(ctx context.Context, id *storiface.ID, dropMissing bool) error //perm:admin

	// SetEnabled marks the worker as enabled/disabled. Not that this setting
	// may take a few seconds to propagate to task scheduler
	SetEnabled(ctx context.Context, enabled bool) error //perm:admin

	Enabled(ctx context.Context) (bool, error) //perm:admin

	// WaitQuiet blocks until there are no tasks running
	WaitQuiet(ctx context.Context) error //perm:admin

	// returns a random UUID of worker session, generated randomly when worker
	// process starts
	ProcessSession(context.Context) (uuid.UUID, error) //perm:admin

	// Like ProcessSession, but returns an error when worker is disabled
	Session(context.Context) (uuid.UUID, error) //perm:admin

	// Trigger shutdown
	Shutdown(context.Context) error //perm:admin

}

var _ storiface.WorkerCalls = *new(Worker)
