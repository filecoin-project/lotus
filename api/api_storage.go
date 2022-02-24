package api

import (
	"bytes"
	"context"
	"time"

	"github.com/filecoin-project/lotus/chain/actors/builtin"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	abinetwork "github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"
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

// StorageMiner is a low-level interface to the Filecoin network storage miner node
type StorageMiner interface {
	Common
	Net

	ActorAddress(context.Context) (address.Address, error) //perm:read

	ActorSectorSize(context.Context, address.Address) (abi.SectorSize, error) //perm:read
	ActorAddressConfig(ctx context.Context) (AddressConfig, error)            //perm:read

	MiningBase(context.Context) (*types.TipSet, error) //perm:read

	// Temp api for testing
	PledgeSector(context.Context) (abi.SectorID, error) //perm:write

	// Get the status of a given sector by ID
	SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (SectorInfo, error) //perm:read

	// Add piece to an open sector. If no sectors with enough space are open,
	// either a new sector will be created, or this call will block until more
	// sectors can be created.
	SectorAddPieceToAny(ctx context.Context, size abi.UnpaddedPieceSize, r storage.Data, d PieceDealInfo) (SectorOffset, error) //perm:admin

	SectorsUnsealPiece(ctx context.Context, sector storage.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd *cid.Cid) error //perm:admin

	// List all staged sectors
	SectorsList(context.Context) ([]abi.SectorNumber, error) //perm:read

	// Get summary info of sectors
	SectorsSummary(ctx context.Context) (map[SectorState]int, error) //perm:read

	// List sectors in particular states
	SectorsListInStates(context.Context, []SectorState) ([]abi.SectorNumber, error) //perm:read

	SectorsRefs(context.Context) (map[string][]SealedRef, error) //perm:read

	// SectorStartSealing can be called on sectors in Empty or WaitDeals states
	// to trigger sealing early
	SectorStartSealing(context.Context, abi.SectorNumber) error //perm:write
	// SectorSetSealDelay sets the time that a newly-created sector
	// waits for more deals before it starts sealing
	SectorSetSealDelay(context.Context, time.Duration) error //perm:write
	// SectorGetSealDelay gets the time that a newly-created sector
	// waits for more deals before it starts sealing
	SectorGetSealDelay(context.Context) (time.Duration, error) //perm:read
	// SectorSetExpectedSealDuration sets the expected time for a sector to seal
	SectorSetExpectedSealDuration(context.Context, time.Duration) error //perm:write
	// SectorGetExpectedSealDuration gets the expected time for a sector to seal
	SectorGetExpectedSealDuration(context.Context) (time.Duration, error) //perm:read
	SectorsUpdate(context.Context, abi.SectorNumber, SectorState) error   //perm:admin
	// SectorRemove removes the sector from storage. It doesn't terminate it on-chain, which can
	// be done with SectorTerminate. Removing and not terminating live sectors will cause additional penalties.
	SectorRemove(context.Context, abi.SectorNumber) error //perm:admin
	// SectorTerminate terminates the sector on-chain (adding it to a termination batch first), then
	// automatically removes it from storage
	SectorTerminate(context.Context, abi.SectorNumber) error //perm:admin
	// SectorTerminateFlush immediately sends a terminate message with sectors batched for termination.
	// Returns null if message wasn't sent
	SectorTerminateFlush(ctx context.Context) (*cid.Cid, error) //perm:admin
	// SectorTerminatePending returns a list of pending sector terminations to be sent in the next batch message
	SectorTerminatePending(ctx context.Context) ([]abi.SectorID, error)             //perm:admin
	SectorMarkForUpgrade(ctx context.Context, id abi.SectorNumber, snap bool) error //perm:admin
	// SectorPreCommitFlush immediately sends a PreCommit message with sectors batched for PreCommit.
	// Returns null if message wasn't sent
	SectorPreCommitFlush(ctx context.Context) ([]sealiface.PreCommitBatchRes, error) //perm:admin
	// SectorPreCommitPending returns a list of pending PreCommit sectors to be sent in the next batch message
	SectorPreCommitPending(ctx context.Context) ([]abi.SectorID, error) //perm:admin
	// SectorCommitFlush immediately sends a Commit message with sectors aggregated for Commit.
	// Returns null if message wasn't sent
	SectorCommitFlush(ctx context.Context) ([]sealiface.CommitBatchRes, error) //perm:admin
	// SectorCommitPending returns a list of pending Commit sectors to be sent in the next aggregate message
	SectorCommitPending(ctx context.Context) ([]abi.SectorID, error) //perm:admin
	SectorMatchPendingPiecesToOpenSectors(ctx context.Context) error //perm:admin
	// SectorAbortUpgrade can be called on sectors that are in the process of being upgraded to abort it
	SectorAbortUpgrade(context.Context, abi.SectorNumber) error //perm:admin

	// WorkerConnect tells the node to connect to workers RPC
	WorkerConnect(context.Context, string) error                              //perm:admin retry:true
	WorkerStats(context.Context) (map[uuid.UUID]storiface.WorkerStats, error) //perm:admin
	WorkerJobs(context.Context) (map[uuid.UUID][]storiface.WorkerJob, error)  //perm:admin

	//storiface.WorkerReturn
	ReturnAddPiece(ctx context.Context, callID storiface.CallID, pi abi.PieceInfo, err *storiface.CallError) error                                      //perm:admin retry:true
	ReturnSealPreCommit1(ctx context.Context, callID storiface.CallID, p1o storage.PreCommit1Out, err *storiface.CallError) error                       //perm:admin retry:true
	ReturnSealPreCommit2(ctx context.Context, callID storiface.CallID, sealed storage.SectorCids, err *storiface.CallError) error                       //perm:admin retry:true
	ReturnSealCommit1(ctx context.Context, callID storiface.CallID, out storage.Commit1Out, err *storiface.CallError) error                             //perm:admin retry:true
	ReturnSealCommit2(ctx context.Context, callID storiface.CallID, proof storage.Proof, err *storiface.CallError) error                                //perm:admin retry:true
	ReturnFinalizeSector(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                  //perm:admin retry:true
	ReturnReplicaUpdate(ctx context.Context, callID storiface.CallID, out storage.ReplicaUpdateOut, err *storiface.CallError) error                     //perm:admin retry:true
	ReturnProveReplicaUpdate1(ctx context.Context, callID storiface.CallID, vanillaProofs storage.ReplicaVanillaProofs, err *storiface.CallError) error //perm:admin retry:true
	ReturnProveReplicaUpdate2(ctx context.Context, callID storiface.CallID, proof storage.ReplicaUpdateProof, err *storiface.CallError) error           //perm:admin retry:true
	ReturnGenerateSectorKeyFromData(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                       //perm:admin retry:true
	ReturnFinalizeReplicaUpdate(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                           //perm:admin retry:true
	ReturnReleaseUnsealed(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                 //perm:admin retry:true
	ReturnMoveStorage(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                     //perm:admin retry:true
	ReturnUnsealPiece(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                     //perm:admin retry:true
	ReturnReadPiece(ctx context.Context, callID storiface.CallID, ok bool, err *storiface.CallError) error                                              //perm:admin retry:true
	ReturnFetch(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                           //perm:admin retry:true

	// SealingSchedDiag dumps internal sealing scheduler state
	SealingSchedDiag(ctx context.Context, doSched bool) (interface{}, error) //perm:admin
	SealingAbort(ctx context.Context, call storiface.CallID) error           //perm:admin

	//stores.SectorIndex
	StorageAttach(context.Context, stores.StorageInfo, fsutil.FsStat) error                                                                                             //perm:admin
	StorageInfo(context.Context, stores.ID) (stores.StorageInfo, error)                                                                                                 //perm:admin
	StorageReportHealth(context.Context, stores.ID, stores.HealthReport) error                                                                                          //perm:admin
	StorageDeclareSector(ctx context.Context, storageID stores.ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error                                     //perm:admin
	StorageDropSector(ctx context.Context, storageID stores.ID, s abi.SectorID, ft storiface.SectorFileType) error                                                      //perm:admin
	StorageFindSector(ctx context.Context, sector abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]stores.SectorStorageInfo, error) //perm:admin
	StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType) ([]stores.StorageInfo, error)           //perm:admin
	StorageLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) error                                          //perm:admin
	StorageTryLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) (bool, error)                               //perm:admin
	StorageList(ctx context.Context) (map[stores.ID][]stores.Decl, error)                                                                                               //perm:admin

	StorageLocal(ctx context.Context) (map[stores.ID]string, error)       //perm:admin
	StorageStat(ctx context.Context, id stores.ID) (fsutil.FsStat, error) //perm:admin

	MarketImportDealData(ctx context.Context, propcid cid.Cid, path string) error                                                                                                        //perm:write
	MarketListDeals(ctx context.Context) ([]MarketDeal, error)                                                                                                                           //perm:read
	MarketListRetrievalDeals(ctx context.Context) ([]retrievalmarket.ProviderDealState, error)                                                                                           //perm:read
	MarketGetDealUpdates(ctx context.Context) (<-chan storagemarket.MinerDeal, error)                                                                                                    //perm:read
	MarketListIncompleteDeals(ctx context.Context) ([]storagemarket.MinerDeal, error)                                                                                                    //perm:read
	MarketSetAsk(ctx context.Context, price types.BigInt, verifiedPrice types.BigInt, duration abi.ChainEpoch, minPieceSize abi.PaddedPieceSize, maxPieceSize abi.PaddedPieceSize) error //perm:admin
	MarketGetAsk(ctx context.Context) (*storagemarket.SignedStorageAsk, error)                                                                                                           //perm:read
	MarketSetRetrievalAsk(ctx context.Context, rask *retrievalmarket.Ask) error                                                                                                          //perm:admin
	MarketGetRetrievalAsk(ctx context.Context) (*retrievalmarket.Ask, error)                                                                                                             //perm:read
	MarketListDataTransfers(ctx context.Context) ([]DataTransferChannel, error)                                                                                                          //perm:write
	MarketDataTransferUpdates(ctx context.Context) (<-chan DataTransferChannel, error)                                                                                                   //perm:write
	// MarketRestartDataTransfer attempts to restart a data transfer with the given transfer ID and other peer
	MarketRestartDataTransfer(ctx context.Context, transferID datatransfer.TransferID, otherPeer peer.ID, isInitiator bool) error //perm:write
	// MarketCancelDataTransfer cancels a data transfer with the given transfer ID and other peer
	MarketCancelDataTransfer(ctx context.Context, transferID datatransfer.TransferID, otherPeer peer.ID, isInitiator bool) error //perm:write
	MarketPendingDeals(ctx context.Context) (PendingDealInfo, error)                                                             //perm:write
	MarketPublishPendingDeals(ctx context.Context) error                                                                         //perm:admin
	MarketRetryPublishDeal(ctx context.Context, propcid cid.Cid) error                                                           //perm:admin

	// DagstoreListShards returns information about all shards known to the
	// DAG store. Only available on nodes running the markets subsystem.
	DagstoreListShards(ctx context.Context) ([]DagstoreShardInfo, error) //perm:read

	// DagstoreInitializeShard initializes an uninitialized shard.
	//
	// Initialization consists of fetching the shard's data (deal payload) from
	// the storage subsystem, generating an index, and persisting the index
	// to facilitate later retrievals, and/or to publish to external sources.
	//
	// This operation is intended to complement the initial migration. The
	// migration registers a shard for every unique piece CID, with lazy
	// initialization. Thus, shards are not initialized immediately to avoid
	// IO activity competing with proving. Instead, shard are initialized
	// when first accessed. This method forces the initialization of a shard by
	// accessing it and immediately releasing it. This is useful to warm up the
	// cache to facilitate subsequent retrievals, and to generate the indexes
	// to publish them externally.
	//
	// This operation fails if the shard is not in ShardStateNew state.
	// It blocks until initialization finishes.
	DagstoreInitializeShard(ctx context.Context, key string) error //perm:write

	// DagstoreRecoverShard attempts to recover a failed shard.
	//
	// This operation fails if the shard is not in ShardStateErrored state.
	// It blocks until recovery finishes. If recovery failed, it returns the
	// error.
	DagstoreRecoverShard(ctx context.Context, key string) error //perm:write

	// DagstoreInitializeAll initializes all uninitialized shards in bulk,
	// according to the policy passed in the parameters.
	//
	// It is recommended to set a maximum concurrency to avoid extreme
	// IO pressure if the storage subsystem has a large amount of deals.
	//
	// It returns a stream of events to report progress.
	DagstoreInitializeAll(ctx context.Context, params DagstoreInitializeAllParams) (<-chan DagstoreInitializeAllEvent, error) //perm:write

	// DagstoreGC runs garbage collection on the DAG store.
	DagstoreGC(ctx context.Context) ([]DagstoreShardResult, error) //perm:admin

	// RuntimeSubsystems returns the subsystems that are enabled
	// in this instance.
	RuntimeSubsystems(ctx context.Context) (MinerSubsystems, error) //perm:read

	DealsImportData(ctx context.Context, dealPropCid cid.Cid, file string) error //perm:admin
	DealsList(ctx context.Context) ([]MarketDeal, error)                         //perm:admin
	DealsConsiderOnlineStorageDeals(context.Context) (bool, error)               //perm:admin
	DealsSetConsiderOnlineStorageDeals(context.Context, bool) error              //perm:admin
	DealsConsiderOnlineRetrievalDeals(context.Context) (bool, error)             //perm:admin
	DealsSetConsiderOnlineRetrievalDeals(context.Context, bool) error            //perm:admin
	DealsPieceCidBlocklist(context.Context) ([]cid.Cid, error)                   //perm:admin
	DealsSetPieceCidBlocklist(context.Context, []cid.Cid) error                  //perm:admin
	DealsConsiderOfflineStorageDeals(context.Context) (bool, error)              //perm:admin
	DealsSetConsiderOfflineStorageDeals(context.Context, bool) error             //perm:admin
	DealsConsiderOfflineRetrievalDeals(context.Context) (bool, error)            //perm:admin
	DealsSetConsiderOfflineRetrievalDeals(context.Context, bool) error           //perm:admin
	DealsConsiderVerifiedStorageDeals(context.Context) (bool, error)             //perm:admin
	DealsSetConsiderVerifiedStorageDeals(context.Context, bool) error            //perm:admin
	DealsConsiderUnverifiedStorageDeals(context.Context) (bool, error)           //perm:admin
	DealsSetConsiderUnverifiedStorageDeals(context.Context, bool) error          //perm:admin

	StorageAddLocal(ctx context.Context, path string) error //perm:admin

	PiecesListPieces(ctx context.Context) ([]cid.Cid, error)                                 //perm:read
	PiecesListCidInfos(ctx context.Context) ([]cid.Cid, error)                               //perm:read
	PiecesGetPieceInfo(ctx context.Context, pieceCid cid.Cid) (*piecestore.PieceInfo, error) //perm:read
	PiecesGetCIDInfo(ctx context.Context, payloadCid cid.Cid) (*piecestore.CIDInfo, error)   //perm:read

	// CreateBackup creates node backup onder the specified file name. The
	// method requires that the lotus-miner is running with the
	// LOTUS_BACKUP_BASE_PATH environment variable set to some path, and that
	// the path specified when calling CreateBackup is within the base path
	CreateBackup(ctx context.Context, fpath string) error //perm:admin

	CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storage.SectorRef, update []bool, expensive bool) (map[abi.SectorNumber]string, error) //perm:admin

	ComputeProof(ctx context.Context, ssi []builtin.ExtendedSectorInfo, rand abi.PoStRandomness, poStEpoch abi.ChainEpoch, nv abinetwork.Version) ([]builtin.PoStProof, error) //perm:read
}

var _ storiface.WorkerReturn = *new(StorageMiner)
var _ stores.SectorIndex = *new(StorageMiner)

type SealRes struct {
	Err   string
	GoErr error `json:"-"`

	Proof []byte
}

type SectorLog struct {
	Kind      string
	Timestamp uint64

	Trace string

	Message string
}

type SectorPiece struct {
	Piece    abi.PieceInfo
	DealInfo *PieceDealInfo // nil for pieces which do not appear in deals (e.g. filler pieces)
}

type SectorInfo struct {
	SectorID     abi.SectorNumber
	State        SectorState
	CommD        *cid.Cid
	CommR        *cid.Cid
	Proof        []byte
	Deals        []abi.DealID
	Pieces       []SectorPiece
	Ticket       SealTicket
	Seed         SealSeed
	PreCommitMsg *cid.Cid
	CommitMsg    *cid.Cid
	Retries      uint64
	ToUpgrade    bool

	LastErr string

	Log []SectorLog

	// On Chain Info
	SealProof          abi.RegisteredSealProof // The seal proof type implies the PoSt proof/s
	Activation         abi.ChainEpoch          // Epoch during which the sector proof was accepted
	Expiration         abi.ChainEpoch          // Epoch during which the sector expires
	DealWeight         abi.DealWeight          // Integral of active deals over sector lifetime
	VerifiedDealWeight abi.DealWeight          // Integral of active verified deals over sector lifetime
	InitialPledge      abi.TokenAmount         // Pledge collected to commit this sector
	// Expiration Info
	OnTime abi.ChainEpoch
	// non-zero if sector is faulty, epoch at which it will be permanently
	// removed if it doesn't recover
	Early abi.ChainEpoch
}

type SealedRef struct {
	SectorID abi.SectorNumber
	Offset   abi.PaddedPieceSize
	Size     abi.UnpaddedPieceSize
}

type SealedRefs struct {
	Refs []SealedRef
}

type SealTicket struct {
	Value abi.SealRandomness
	Epoch abi.ChainEpoch
}

type SealSeed struct {
	Value abi.InteractiveSealRandomness
	Epoch abi.ChainEpoch
}

func (st *SealTicket) Equals(ost *SealTicket) bool {
	return bytes.Equal(st.Value, ost.Value) && st.Epoch == ost.Epoch
}

func (st *SealSeed) Equals(ost *SealSeed) bool {
	return bytes.Equal(st.Value, ost.Value) && st.Epoch == ost.Epoch
}

type SectorState string

type AddrUse int

const (
	PreCommitAddr AddrUse = iota
	CommitAddr
	DealPublishAddr
	PoStAddr

	TerminateSectorsAddr
)

type AddressConfig struct {
	PreCommitControl   []address.Address
	CommitControl      []address.Address
	TerminateControl   []address.Address
	DealPublishControl []address.Address

	DisableOwnerFallback  bool
	DisableWorkerFallback bool
}

// PendingDealInfo has info about pending deals and when they are due to be
// published
type PendingDealInfo struct {
	Deals              []market.ClientDealProposal
	PublishPeriodStart time.Time
	PublishPeriod      time.Duration
}

type SectorOffset struct {
	Sector abi.SectorNumber
	Offset abi.PaddedPieceSize
}

// DealInfo is a tuple of deal identity and its schedule
type PieceDealInfo struct {
	PublishCid   *cid.Cid
	DealID       abi.DealID
	DealProposal *market.DealProposal
	DealSchedule DealSchedule
	KeepUnsealed bool
}

// DealSchedule communicates the time interval of a storage deal. The deal must
// appear in a sealed (proven) sector no later than StartEpoch, otherwise it
// is invalid.
type DealSchedule struct {
	StartEpoch abi.ChainEpoch
	EndEpoch   abi.ChainEpoch
}

// DagstoreShardInfo is the serialized form of dagstore.DagstoreShardInfo that
// we expose through JSON-RPC to avoid clients having to depend on the
// dagstore lib.
type DagstoreShardInfo struct {
	Key   string
	State string
	Error string
}

// DagstoreShardResult enumerates results per shard.
type DagstoreShardResult struct {
	Key     string
	Success bool
	Error   string
}

type DagstoreInitializeAllParams struct {
	MaxConcurrency int
	IncludeSealed  bool
}

// DagstoreInitializeAllEvent represents an initialization event.
type DagstoreInitializeAllEvent struct {
	Key     string
	Event   string // "start", "end"
	Success bool
	Error   string
	Total   int
	Current int
}
