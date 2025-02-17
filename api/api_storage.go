package api

import (
	"bytes"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	abinetwork "github.com/filecoin-project/go-state-types/network"

	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
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

// StorageMiner is a low-level interface to the Filecoin network storage miner node
type StorageMiner interface {
	Common

	ActorAddress(context.Context) (address.Address, error) //perm:read

	ActorSectorSize(context.Context, address.Address) (abi.SectorSize, error) //perm:read
	ActorAddressConfig(ctx context.Context) (AddressConfig, error)            //perm:read

	// ActorWithdrawBalance allows to withdraw balance from miner actor to owner address
	// Specify amount as "0" to withdraw full balance. This method returns a message CID
	// and does not wait for message execution
	ActorWithdrawBalance(ctx context.Context, amount abi.TokenAmount) (cid.Cid, error) //perm:admin

	// BeneficiaryWithdrawBalance allows the beneficiary of a miner to withdraw balance from miner actor
	// Specify amount as "0" to withdraw full balance. This method returns a message CID
	// and does not wait for message execution
	BeneficiaryWithdrawBalance(context.Context, abi.TokenAmount) (cid.Cid, error) //perm:admin

	MiningBase(context.Context) (*types.TipSet, error) //perm:read

	ComputeWindowPoSt(ctx context.Context, dlIdx uint64, tsk types.TipSetKey) ([]miner.SubmitWindowedPoStParams, error) //perm:admin

	ComputeDataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (abi.PieceInfo, error) //perm:admin

	// Temp api for testing
	PledgeSector(context.Context) (abi.SectorID, error) //perm:write

	// Get the status of a given sector by ID
	SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (SectorInfo, error) //perm:read

	// Add piece to an open sector. If no sectors with enough space are open,
	// either a new sector will be created, or this call will block until more
	// sectors can be created.
	SectorAddPieceToAny(ctx context.Context, size abi.UnpaddedPieceSize, r storiface.Data, d piece.PieceDealInfo) (SectorOffset, error) //perm:admin

	SectorsUnsealPiece(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd *cid.Cid) error //perm:admin

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
	SectorRemove(context.Context, abi.SectorNumber) error                           //perm:admin
	SectorMarkForUpgrade(ctx context.Context, id abi.SectorNumber, snap bool) error //perm:admin
	// SectorTerminate terminates the sector on-chain (adding it to a termination batch first), then
	// automatically removes it from storage
	SectorTerminate(context.Context, abi.SectorNumber) error //perm:admin
	// SectorTerminateFlush immediately sends a terminate message with sectors batched for termination.
	// Returns null if message wasn't sent
	SectorTerminateFlush(ctx context.Context) (*cid.Cid, error) //perm:admin
	// SectorTerminatePending returns a list of pending sector terminations to be sent in the next batch message
	SectorTerminatePending(ctx context.Context) ([]abi.SectorID, error) //perm:admin
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
	// SectorUnseal unseals the provided sector
	SectorUnseal(ctx context.Context, number abi.SectorNumber) error //perm:admin

	// SectorNumAssignerMeta returns sector number assigner metadata - reserved/allocated
	SectorNumAssignerMeta(ctx context.Context) (NumAssignerMeta, error) //perm:read
	// SectorNumReservations returns a list of sector number reservations
	SectorNumReservations(ctx context.Context) (map[string]bitfield.BitField, error) //perm:read
	// SectorNumReserve creates a new sector number reservation. Will fail if any other reservation has colliding
	// numbers or name. Set force to true to override safety checks.
	// Valid characters for name: a-z, A-Z, 0-9, _, -
	SectorNumReserve(ctx context.Context, name string, sectors bitfield.BitField, force bool) error //perm:admin
	// SectorNumReserveCount creates a new sector number reservation for `count` sector numbers.
	// by default lotus will allocate lowest-available sector numbers to the reservation.
	// For restrictions on `name` see SectorNumReserve
	SectorNumReserveCount(ctx context.Context, name string, count uint64) (bitfield.BitField, error) //perm:admin
	// SectorNumFree drops a sector reservation
	SectorNumFree(ctx context.Context, name string) error //perm:admin

	SectorReceive(ctx context.Context, meta RemoteSectorMeta) error //perm:admin

	// WorkerConnect tells the node to connect to workers RPC
	WorkerConnect(context.Context, string) error                              //perm:admin retry:true
	WorkerStats(context.Context) (map[uuid.UUID]storiface.WorkerStats, error) //perm:admin
	WorkerJobs(context.Context) (map[uuid.UUID][]storiface.WorkerJob, error)  //perm:admin

	// storiface.WorkerReturn
	ReturnDataCid(ctx context.Context, callID storiface.CallID, pi abi.PieceInfo, err *storiface.CallError) error                                         //perm:admin retry:true
	ReturnAddPiece(ctx context.Context, callID storiface.CallID, pi abi.PieceInfo, err *storiface.CallError) error                                        //perm:admin retry:true
	ReturnSealPreCommit1(ctx context.Context, callID storiface.CallID, p1o storiface.PreCommit1Out, err *storiface.CallError) error                       //perm:admin retry:true
	ReturnSealPreCommit2(ctx context.Context, callID storiface.CallID, sealed storiface.SectorCids, err *storiface.CallError) error                       //perm:admin retry:true
	ReturnSealCommit1(ctx context.Context, callID storiface.CallID, out storiface.Commit1Out, err *storiface.CallError) error                             //perm:admin retry:true
	ReturnSealCommit2(ctx context.Context, callID storiface.CallID, proof storiface.Proof, err *storiface.CallError) error                                //perm:admin retry:true
	ReturnFinalizeSector(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                    //perm:admin retry:true
	ReturnReplicaUpdate(ctx context.Context, callID storiface.CallID, out storiface.ReplicaUpdateOut, err *storiface.CallError) error                     //perm:admin retry:true
	ReturnProveReplicaUpdate1(ctx context.Context, callID storiface.CallID, vanillaProofs storiface.ReplicaVanillaProofs, err *storiface.CallError) error //perm:admin retry:true
	ReturnProveReplicaUpdate2(ctx context.Context, callID storiface.CallID, proof storiface.ReplicaUpdateProof, err *storiface.CallError) error           //perm:admin retry:true
	ReturnGenerateSectorKeyFromData(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                         //perm:admin retry:true
	ReturnFinalizeReplicaUpdate(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                             //perm:admin retry:true
	ReturnReleaseUnsealed(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                   //perm:admin retry:true
	ReturnMoveStorage(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                       //perm:admin retry:true
	ReturnUnsealPiece(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                       //perm:admin retry:true
	ReturnReadPiece(ctx context.Context, callID storiface.CallID, ok bool, err *storiface.CallError) error                                                //perm:admin retry:true
	ReturnDownloadSector(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                    //perm:admin retry:true
	ReturnFetch(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error                                                             //perm:admin retry:true

	// SealingSchedDiag dumps internal sealing scheduler state
	SealingSchedDiag(ctx context.Context, doSched bool) (interface{}, error) //perm:admin
	SealingAbort(ctx context.Context, call storiface.CallID) error           //perm:admin
	// SealingRemoveRequest removes a request from sealing pipeline
	SealingRemoveRequest(ctx context.Context, schedId uuid.UUID) error //perm:admin

	// paths.SectorIndex
	StorageAttach(context.Context, storiface.StorageInfo, fsutil.FsStat) error                                                         //perm:admin
	StorageDetach(ctx context.Context, id storiface.ID, url string) error                                                              //perm:admin
	StorageInfo(context.Context, storiface.ID) (storiface.StorageInfo, error)                                                          //perm:admin
	StorageReportHealth(context.Context, storiface.ID, storiface.HealthReport) error                                                   //perm:admin
	StorageDeclareSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error //perm:admin
	StorageDropSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType) error                  //perm:admin
	// StorageFindSector returns list of paths where the specified sector files exist.
	//
	// If allowFetch is set, list of paths to which the sector can be fetched will also be returned.
	// - Paths which have sector files locally (don't require fetching) will be listed first.
	// - Paths which have sector files locally will not be filtered based on AllowTypes/DenyTypes.
	// - Paths which require fetching will be filtered based on AllowTypes/DenyTypes. If multiple
	//   file types are specified, each type will be considered individually, and a union of all paths
	//   which can accommodate each file type will be returned.
	StorageFindSector(ctx context.Context, sector abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error) //perm:admin
	// StorageBestAlloc returns list of paths where sector files of the specified type can be allocated, ordered by preference.
	// Paths with more weight and more % of free space are preferred.
	// Note: This method doesn't filter paths based on AllowTypes/DenyTypes.
	StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType, miner abi.ActorID) ([]storiface.StorageInfo, error) //perm:admin
	StorageLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) error                                                      //perm:admin
	StorageTryLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) (bool, error)                                           //perm:admin
	StorageList(ctx context.Context) (map[storiface.ID][]storiface.Decl, error)                                                                                                     //perm:admin
	StorageGetLocks(ctx context.Context) (storiface.SectorLocks, error)                                                                                                             //perm:admin

	StorageLocal(ctx context.Context) (map[storiface.ID]string, error)       //perm:admin
	StorageStat(ctx context.Context, id storiface.ID) (fsutil.FsStat, error) //perm:admin

	StorageAuthVerify(ctx context.Context, token string) ([]auth.Permission, error) //perm:read

	StorageAddLocal(ctx context.Context, path string) error                              //perm:admin
	StorageDetachLocal(ctx context.Context, path string) error                           //perm:admin
	StorageRedeclareLocal(ctx context.Context, id *storiface.ID, dropMissing bool) error //perm:admin

	MarketListDeals(ctx context.Context) ([]*MarketDeal, error) //perm:read

	// RuntimeSubsystems returns the subsystems that are enabled
	// in this instance.
	RuntimeSubsystems(ctx context.Context) (MinerSubsystems, error) //perm:read

	// CreateBackup creates node backup under the specified file name. The
	// method requires that the lotus-miner is running with the
	// LOTUS_BACKUP_BASE_PATH environment variable set to some path, and that
	// the path specified when calling CreateBackup is within the base path
	CreateBackup(ctx context.Context, fpath string) error //perm:admin

	CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storiface.SectorRef) (map[abi.SectorNumber]string, error) //perm:admin

	ComputeProof(ctx context.Context, ssi []builtinactors.ExtendedSectorInfo, rand abi.PoStRandomness, poStEpoch abi.ChainEpoch, nv abinetwork.Version) ([]builtinactors.PoStProof, error) //perm:read

	// RecoverFault can be used to declare recoveries manually. It sends messages
	// to the miner actor with details of recovered sectors and returns the CID of messages. It honors the
	// maxPartitionsPerRecoveryMessage from the config
	RecoverFault(ctx context.Context, sectors []abi.SectorNumber) ([]cid.Cid, error) //perm:admin
}

var _ storiface.WorkerReturn = *new(StorageMiner)

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
	Piece abi.PieceInfo

	// DealInfo is nil for pieces which do not appear in deals (e.g. filler pieces)
	// NOTE: DDO pieces which aren't associated with a market deal and have no
	// verified allocation will still have a non-nil DealInfo.
	// nil DealInfo indicates that the piece is a filler, and has zero piece commitment.
	DealInfo *piece.PieceDealInfo
}

// PieceDealInfo is deprecated
//
// DEPRECATED: Use piece.PieceDealInfo instead
type PieceDealInfo = piece.PieceDealInfo

// DealSchedule is deprecated
//
// DEPRECATED: Use piece.DealSchedule instead
type DealSchedule = piece.DealSchedule

type SectorInfo struct {
	SectorID             abi.SectorNumber
	State                SectorState
	CommD                *cid.Cid
	CommR                *cid.Cid
	Proof                []byte
	Deals                []abi.DealID
	Pieces               []SectorPiece
	Ticket               SealTicket
	Seed                 SealSeed
	PreCommitMsg         *cid.Cid
	CommitMsg            *cid.Cid
	Retries              uint64
	ToUpgrade            bool
	ReplicaUpdateMessage *cid.Cid

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

func (s *SectorState) String() string {
	return string(*s)
}

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

type NumAssignerMeta struct {
	Reserved  bitfield.BitField
	Allocated bitfield.BitField

	// ChainAllocated+Reserved+Allocated
	InUse bitfield.BitField

	Next abi.SectorNumber
}

type RemoteSectorMeta struct {
	////////
	// BASIC SECTOR INFORMATION

	// State specifies the first state the sector will enter after being imported
	// Must be one of the following states:
	// * Packing
	// * GetTicket
	// * PreCommitting
	// * SubmitCommit
	// * Proving/Available
	State SectorState

	Sector abi.SectorID
	Type   abi.RegisteredSealProof

	////////
	// SEALING METADATA
	// (allows lotus to continue the sealing process)

	// Required in Packing and later
	Pieces []SectorPiece // todo better type?

	// Required in PreCommitting and later
	TicketValue   abi.SealRandomness
	TicketEpoch   abi.ChainEpoch
	PreCommit1Out storiface.PreCommit1Out // todo specify better

	CommD *cid.Cid
	CommR *cid.Cid // SectorKey

	// Required in SubmitCommit and later
	PreCommitInfo    *miner.SectorPreCommitInfo
	PreCommitDeposit *big.Int
	PreCommitMessage *cid.Cid
	PreCommitTipSet  types.TipSetKey

	SeedValue abi.InteractiveSealRandomness
	SeedEpoch abi.ChainEpoch

	CommitProof []byte

	// Required in Proving/Available
	CommitMessage *cid.Cid

	// Optional sector metadata to import
	Log []SectorLog

	////////
	// SECTOR DATA SOURCE

	// Sector urls - lotus will use those for fetching files into local storage

	// Required in all states
	DataUnsealed *storiface.SectorLocation

	// Required in PreCommitting and later
	DataSealed *storiface.SectorLocation
	DataCache  *storiface.SectorLocation

	////////
	// SEALING SERVICE HOOKS

	// URL
	// RemoteCommit1Endpoint is an URL of POST endpoint which lotus will call requesting Commit1 (seal_commit_phase1)
	// request body will be json-serialized RemoteCommit1Params struct
	RemoteCommit1Endpoint string

	// RemoteCommit2Endpoint is an URL of POST endpoint which lotus will call requesting Commit2 (seal_commit_phase2)
	// request body will be json-serialized RemoteCommit2Params struct
	RemoteCommit2Endpoint string

	// RemoteSealingDoneEndpoint is called after the sector exists the sealing pipeline
	// request body will be json-serialized RemoteSealingDoneParams struct
	RemoteSealingDoneEndpoint string
}

type RemoteCommit1Params struct {
	Ticket, Seed []byte

	Unsealed cid.Cid
	Sealed   cid.Cid

	ProofType abi.RegisteredSealProof
}

type RemoteCommit2Params struct {
	Sector    abi.SectorID
	ProofType abi.RegisteredSealProof

	// todo spec better
	Commit1Out storiface.Commit1Out
}

type RemoteSealingDoneParams struct {
	// Successful is true if the sector has entered state considered as "successfully sealed"
	Successful bool

	// State is the state the sector has entered
	// For example "Proving" / "Removing"
	State string

	// Optional commit message CID
	CommitMessage *cid.Cid
}
