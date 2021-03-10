package api

import (
	"bytes"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

// StorageMiner is a low-level interface to the Filecoin network storage miner node
type StorageMiner interface {
	Common

	ActorAddress(context.Context) (address.Address, error)

	ActorSectorSize(context.Context, address.Address) (abi.SectorSize, error)
	ActorAddressConfig(ctx context.Context) (AddressConfig, error)

	MiningBase(context.Context) (*types.TipSet, error)

	// Temp api for testing
	PledgeSector(context.Context) (abi.SectorID, error)

	// Get the status of a given sector by ID
	SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (SectorInfo, error)

	// List all staged sectors
	SectorsList(context.Context) ([]abi.SectorNumber, error)

	// Get summary info of sectors
	SectorsSummary(ctx context.Context) (map[SectorState]int, error)

	// List sectors in particular states
	SectorsListInStates(context.Context, []SectorState) ([]abi.SectorNumber, error)

	SectorsRefs(context.Context) (map[string][]SealedRef, error)

	// SectorStartSealing can be called on sectors in Empty or WaitDeals states
	// to trigger sealing early
	SectorStartSealing(context.Context, abi.SectorNumber) error
	// SectorSetSealDelay sets the time that a newly-created sector
	// waits for more deals before it starts sealing
	SectorSetSealDelay(context.Context, time.Duration) error
	// SectorGetSealDelay gets the time that a newly-created sector
	// waits for more deals before it starts sealing
	SectorGetSealDelay(context.Context) (time.Duration, error)
	// SectorSetExpectedSealDuration sets the expected time for a sector to seal
	SectorSetExpectedSealDuration(context.Context, time.Duration) error
	// SectorGetExpectedSealDuration gets the expected time for a sector to seal
	SectorGetExpectedSealDuration(context.Context) (time.Duration, error)
	SectorsUpdate(context.Context, abi.SectorNumber, SectorState) error
	// SectorRemove removes the sector from storage. It doesn't terminate it on-chain, which can
	// be done with SectorTerminate. Removing and not terminating live sectors will cause additional penalties.
	SectorRemove(context.Context, abi.SectorNumber) error
	// SectorTerminate terminates the sector on-chain (adding it to a termination batch first), then
	// automatically removes it from storage
	SectorTerminate(context.Context, abi.SectorNumber) error
	// SectorTerminateFlush immediately sends a terminate message with sectors batched for termination.
	// Returns null if message wasn't sent
	SectorTerminateFlush(ctx context.Context) (*cid.Cid, error)
	// SectorTerminatePending returns a list of pending sector terminations to be sent in the next batch message
	SectorTerminatePending(ctx context.Context) ([]abi.SectorID, error)
	SectorMarkForUpgrade(ctx context.Context, id abi.SectorNumber) error

	StorageList(ctx context.Context) (map[stores.ID][]stores.Decl, error)
	StorageLocal(ctx context.Context) (map[stores.ID]string, error)
	StorageStat(ctx context.Context, id stores.ID) (fsutil.FsStat, error)

	// WorkerConnect tells the node to connect to workers RPC
	WorkerConnect(context.Context, string) error
	WorkerStats(context.Context) (map[uuid.UUID]storiface.WorkerStats, error)
	WorkerJobs(context.Context) (map[uuid.UUID][]storiface.WorkerJob, error)
	storiface.WorkerReturn

	// SealingSchedDiag dumps internal sealing scheduler state
	SealingSchedDiag(ctx context.Context, doSched bool) (interface{}, error)
	SealingAbort(ctx context.Context, call storiface.CallID) error

	stores.SectorIndex

	MarketImportDealData(ctx context.Context, propcid cid.Cid, path string) error
	MarketListDeals(ctx context.Context) ([]MarketDeal, error)
	MarketListRetrievalDeals(ctx context.Context) ([]retrievalmarket.ProviderDealState, error)
	MarketGetDealUpdates(ctx context.Context) (<-chan storagemarket.MinerDeal, error)
	MarketListIncompleteDeals(ctx context.Context) ([]storagemarket.MinerDeal, error)
	MarketSetAsk(ctx context.Context, price types.BigInt, verifiedPrice types.BigInt, duration abi.ChainEpoch, minPieceSize abi.PaddedPieceSize, maxPieceSize abi.PaddedPieceSize) error
	MarketGetAsk(ctx context.Context) (*storagemarket.SignedStorageAsk, error)
	MarketSetRetrievalAsk(ctx context.Context, rask *retrievalmarket.Ask) error
	MarketGetRetrievalAsk(ctx context.Context) (*retrievalmarket.Ask, error)
	MarketListDataTransfers(ctx context.Context) ([]DataTransferChannel, error)
	MarketDataTransferUpdates(ctx context.Context) (<-chan DataTransferChannel, error)
	// MarketRestartDataTransfer attempts to restart a data transfer with the given transfer ID and other peer
	MarketRestartDataTransfer(ctx context.Context, transferID datatransfer.TransferID, otherPeer peer.ID, isInitiator bool) error
	// MarketCancelDataTransfer cancels a data transfer with the given transfer ID and other peer
	MarketCancelDataTransfer(ctx context.Context, transferID datatransfer.TransferID, otherPeer peer.ID, isInitiator bool) error
	MarketPendingDeals(ctx context.Context) (PendingDealInfo, error)
	MarketPublishPendingDeals(ctx context.Context) error

	DealsImportData(ctx context.Context, dealPropCid cid.Cid, file string) error
	DealsList(ctx context.Context) ([]MarketDeal, error)
	DealsConsiderOnlineStorageDeals(context.Context) (bool, error)
	DealsSetConsiderOnlineStorageDeals(context.Context, bool) error
	DealsConsiderOnlineRetrievalDeals(context.Context) (bool, error)
	DealsSetConsiderOnlineRetrievalDeals(context.Context, bool) error
	DealsPieceCidBlocklist(context.Context) ([]cid.Cid, error)
	DealsSetPieceCidBlocklist(context.Context, []cid.Cid) error
	DealsConsiderOfflineStorageDeals(context.Context) (bool, error)
	DealsSetConsiderOfflineStorageDeals(context.Context, bool) error
	DealsConsiderOfflineRetrievalDeals(context.Context) (bool, error)
	DealsSetConsiderOfflineRetrievalDeals(context.Context, bool) error
	DealsConsiderVerifiedStorageDeals(context.Context) (bool, error)
	DealsSetConsiderVerifiedStorageDeals(context.Context, bool) error
	DealsConsiderUnverifiedStorageDeals(context.Context) (bool, error)
	DealsSetConsiderUnverifiedStorageDeals(context.Context, bool) error

	StorageAddLocal(ctx context.Context, path string) error

	PiecesListPieces(ctx context.Context) ([]cid.Cid, error)
	PiecesListCidInfos(ctx context.Context) ([]cid.Cid, error)
	PiecesGetPieceInfo(ctx context.Context, pieceCid cid.Cid) (*piecestore.PieceInfo, error)
	PiecesGetCIDInfo(ctx context.Context, payloadCid cid.Cid) (*piecestore.CIDInfo, error)

	// CreateBackup creates node backup onder the specified file name. The
	// method requires that the lotus-miner is running with the
	// LOTUS_BACKUP_BASE_PATH environment variable set to some path, and that
	// the path specified when calling CreateBackup is within the base path
	CreateBackup(ctx context.Context, fpath string) error

	CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storage.SectorRef, expensive bool) (map[abi.SectorNumber]string, error)
}

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

type SectorInfo struct {
	SectorID     abi.SectorNumber
	State        SectorState
	CommD        *cid.Cid
	CommR        *cid.Cid
	Proof        []byte
	Deals        []abi.DealID
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
	PoStAddr

	TerminateSectorsAddr
)

type AddressConfig struct {
	PreCommitControl []address.Address
	CommitControl    []address.Address
	TerminateControl []address.Address

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
