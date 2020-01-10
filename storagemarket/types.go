package storagemarket

import (
	"context"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

const DealProtocolID = "/fil/storage/mk/1.0.1"
const AskProtocolID = "/fil/storage/ask/1.0.1"

// type shims - used during migration into separate module
type Balance = actors.StorageParticipantBalance
type DealID uint64
type Signature = types.Signature
type StorageDeal = actors.OnChainDeal
type StorageAsk = types.SignedStorageAsk
type StateKey = *types.TipSet
type Epoch uint64
type TokenAmount BigInt

// Duplicated from deals package for now
type MinerDeal struct {
	ProposalCid cid.Cid
	Proposal    actors.StorageDealProposal
	Miner       peer.ID
	Client      peer.ID
	State       api.DealState

	Ref cid.Cid

	DealID   uint64
	SectorID uint64 // Set when sm >= DealStaged
}

type ClientDeal struct {
	ProposalCid    cid.Cid
	Proposal       actors.StorageDealProposal
	State          api.DealState
	Miner          peer.ID
	MinerWorker    address.Address
	DealID         uint64
	PayloadCid     cid.Cid
	PublishMessage *cid.Cid
}

// The interface provided for storage providers
type StorageProvider interface {
	Run(ctx context.Context, host host.Host)

	Stop()

	AddAsk(price TokenAmount, ttlsecs int64) error

	// ListAsks lists current asks
	ListAsks(addr address.Address) []*StorageAsk

	// ListDeals lists on-chain deals associated with this provider
	ListDeals(ctx context.Context) ([]StorageDeal, error)

	// ListIncompleteDeals lists deals that are in progress or rejected
	ListIncompleteDeals() ([]MinerDeal, error)

	// AddStorageCollateral adds storage collateral
	AddStorageCollateral(ctx context.Context, amount TokenAmount) error

	// GetStorageCollateral returns the current collateral balance
	GetStorageCollateral(ctx context.Context) (Balance, error)
}

// Node dependencies for a StorageProvider
type StorageProviderNode interface {
	MostRecentStateId(ctx context.Context) (StateKey, error)

	// Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
	AddFunds(ctx context.Context, addr address.Address, amount TokenAmount) error

	// Ensures that a storage market participant has a certain amount of available funds
	EnsureFunds(ctx context.Context, addr address.Address, amount TokenAmount) error

	// GetBalance returns locked/unlocked for a storage participant.  Used by both providers and clients.
	GetBalance(ctx context.Context, addr address.Address) (Balance, error)

	// Publishes deal on chain
	PublishDeals(ctx context.Context, deal MinerDeal) (DealID, cid.Cid, error)

	// ListProviderDeals lists all deals associated with a storage provider
	ListProviderDeals(ctx context.Context, addr address.Address) ([]StorageDeal, error)

	// Called when a deal is complete and on chain, and data has been transferred and is ready to be added to a sector
	// returns sector id
	OnDealComplete(ctx context.Context, deal MinerDeal, piecePath string) (uint64, error)

	// returns the worker address associated with a miner
	GetMinerWorker(ctx context.Context, miner address.Address) (address.Address, error)

	// Signs bytes
	SignBytes(ctx context.Context, signer address.Address, b []byte) (*types.Signature, error)
}

type DealSectorCommittedCallback func(error)

// Node dependencies for a StorageClient
type StorageClientNode interface {
	MostRecentStateId(ctx context.Context) (StateKey, error)

	// Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
	AddFunds(ctx context.Context, addr address.Address, amount TokenAmount) error

	EnsureFunds(ctx context.Context, addr address.Address, amount TokenAmount) error

	// GetBalance returns locked/unlocked for a storage participant.  Used by both providers and clients.
	GetBalance(ctx context.Context, addr address.Address) (Balance, error)

	//// ListClientDeals lists all on-chain deals associated with a storage client
	ListClientDeals(ctx context.Context, addr address.Address) ([]StorageDeal, error)

	// GetProviderInfo returns information about a single storage provider
	//GetProviderInfo(stateId StateID, addr Address) *StorageProviderInfo

	// GetStorageProviders returns information about known miners
	ListStorageProviders(ctx context.Context) ([]*StorageProviderInfo, error)

	// Subscribes to storage market actor state changes for a given address.
	// TODO: Should there be a timeout option for this?  In the case that we are waiting for funds to be deposited and it never happens?
	//SubscribeStorageMarketEvents(addr Address, handler StorageMarketEventHandler) (SubID, error)

	// Cancels a subscription
	//UnsubscribeStorageMarketEvents(subId SubID)
	ValidatePublishedDeal(ctx context.Context, deal ClientDeal) (uint64, error)

	// SignProposal signs a proposal
	SignProposal(ctx context.Context, signer address.Address, proposal *actors.StorageDealProposal) error

	GetDefaultWalletAddress(ctx context.Context) (address.Address, error)

	OnDealSectorCommitted(ctx context.Context, provider address.Address, dealId uint64, cb DealSectorCommittedCallback) error

	ValidateAskSignature(ask *StorageAsk) error
}

type StorageClientProofs interface {
	//GeneratePieceCommitment(piece io.Reader, pieceSize uint64) (CommP, error)
}

// Closely follows the MinerInfo struct in the spec
type StorageProviderInfo struct {
	Address    address.Address // actor address
	Owner      address.Address
	Worker     address.Address // signs messages
	SectorSize uint64
	PeerID     peer.ID
	// probably more like how much storage power, available collateral etc
}

type ProposeStorageDealResult struct {
	ProposalCid cid.Cid
}

// The interface provided by the module to the outside world for storage clients.
type StorageClient interface {
	Run(ctx context.Context)

	Stop()

	// ListProviders queries chain state and returns active storage providers
	ListProviders(ctx context.Context) (<-chan StorageProviderInfo, error)

	// ListDeals lists on-chain deals associated with this provider
	ListDeals(ctx context.Context, addr address.Address) ([]StorageDeal, error)

	// ListInProgressDeals lists deals that are in progress or rejected
	ListInProgressDeals(ctx context.Context) ([]ClientDeal, error)

	// ListInProgressDeals lists deals that are in progress or rejected
	GetInProgressDeal(ctx context.Context, cid cid.Cid) (ClientDeal, error)

	// GetAsk returns the current ask for a storage provider
	GetAsk(ctx context.Context, info StorageProviderInfo) (*StorageAsk, error)

	//// FindStorageOffers lists providers and queries them to find offers that satisfy some criteria based on price, duration, etc.
	//FindStorageOffers(criteria AskCriteria, limit uint) []*StorageOffer

	// ProposeStorageDeal initiates deal negotiation with a Storage Provider
	ProposeStorageDeal(ctx context.Context, addr address.Address, info *StorageProviderInfo, payloadCid cid.Cid, proposalExpiration Epoch, duration Epoch, price TokenAmount, collateral TokenAmount) (*ProposeStorageDealResult, error)

	// GetPaymentEscrow returns the current funds available for deal payment
	GetPaymentEscrow(ctx context.Context, addr address.Address) (Balance, error)

	// AddStorageCollateral adds storage collateral
	AddPaymentEscrow(ctx context.Context, addr address.Address, amount TokenAmount) error
}
