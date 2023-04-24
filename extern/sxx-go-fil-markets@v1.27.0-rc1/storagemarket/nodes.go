package storagemarket

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v8/verifreg"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/go-fil-markets/shared"
)

// DealSectorPreCommittedCallback is a callback that runs when a sector is pre-committed
// sectorNumber: the number of the sector that the deal is in
// isActive: the deal is already active
type DealSectorPreCommittedCallback func(sectorNumber abi.SectorNumber, isActive bool, err error)

// DealSectorCommittedCallback is a callback that runs when a sector is committed
type DealSectorCommittedCallback func(err error)

// DealExpiredCallback is a callback that runs when a deal expires
type DealExpiredCallback func(err error)

// DealSlashedCallback is a callback that runs when a deal gets slashed
type DealSlashedCallback func(slashEpoch abi.ChainEpoch, err error)

// StorageCommon are common interfaces provided by a filecoin Node to both StorageClient and StorageProvider
type StorageCommon interface {

	// GetChainHead returns a tipset token for the current chain head
	GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)

	// Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
	AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) (cid.Cid, error)

	// ReserveFunds reserves the given amount of funds is ensures it is available for the deal
	ReserveFunds(ctx context.Context, wallet, addr address.Address, amt abi.TokenAmount) (cid.Cid, error)

	// ReleaseFunds releases funds reserved with ReserveFunds
	ReleaseFunds(ctx context.Context, addr address.Address, amt abi.TokenAmount) error

	// GetBalance returns locked/unlocked for a storage participant.  Used by both providers and clients.
	GetBalance(ctx context.Context, addr address.Address, tok shared.TipSetToken) (Balance, error)

	// VerifySignature verifies a given set of data was signed properly by a given address's private key
	VerifySignature(ctx context.Context, signature crypto.Signature, signer address.Address, plaintext []byte, tok shared.TipSetToken) (bool, error)

	// WaitForMessage waits until a message appears on chain. If it is already on chain, the callback is called immediately
	WaitForMessage(ctx context.Context, mcid cid.Cid, onCompletion func(exitcode.ExitCode, []byte, cid.Cid, error) error) error

	// SignsBytes signs the given data with the given address's private key
	SignBytes(ctx context.Context, signer address.Address, b []byte) (*crypto.Signature, error)

	// DealProviderCollateralBounds returns the min and max collateral a storage provider can issue.
	DealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, isVerified bool) (abi.TokenAmount, abi.TokenAmount, error)

	// OnDealSectorPreCommitted waits for a deal's sector to be pre-committed
	OnDealSectorPreCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, proposal market.DealProposal, publishCid *cid.Cid, cb DealSectorPreCommittedCallback) error

	// OnDealSectorCommitted waits for a deal's sector to be sealed and proved, indicating the deal is active
	OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, sectorNumber abi.SectorNumber, proposal market.DealProposal, publishCid *cid.Cid, cb DealSectorCommittedCallback) error

	// OnDealExpiredOrSlashed registers callbacks to be called when the deal expires or is slashed
	OnDealExpiredOrSlashed(ctx context.Context, dealID abi.DealID, onDealExpired DealExpiredCallback, onDealSlashed DealSlashedCallback) error
}

// PackingResult returns information about how a deal was put into a sector
type PackingResult struct {
	SectorNumber abi.SectorNumber
	Offset       abi.PaddedPieceSize
	Size         abi.PaddedPieceSize
}

// PublishDealsWaitResult is the result of a call to wait for publish deals to
// appear on chain
type PublishDealsWaitResult struct {
	DealID   abi.DealID
	FinalCid cid.Cid
}

// StorageProviderNode are node dependencies for a StorageProvider
type StorageProviderNode interface {
	StorageCommon

	// PublishDeals publishes a deal on chain, returns the message cid, but does not wait for message to appear
	PublishDeals(ctx context.Context, deal MinerDeal) (cid.Cid, error)

	// WaitForPublishDeals waits for a deal publish message to land on chain.
	WaitForPublishDeals(ctx context.Context, mcid cid.Cid, proposal market.DealProposal) (*PublishDealsWaitResult, error)

	// OnDealComplete is called when a deal is complete and on chain, and data has been transferred and is ready to be added to a sector
	OnDealComplete(ctx context.Context, deal MinerDeal, pieceSize abi.UnpaddedPieceSize, pieceReader shared.ReadSeekStarter) (*PackingResult, error)

	// add by lin
	OnDealCompleteOfSxx(ctx context.Context, deal MinerDeal, pieceSize abi.UnpaddedPieceSize, pieceReader shared.ReadSeekStarter) (*PackingResult, error)
	// end

	// GetMinerWorkerAddress returns the worker address associated with a miner
	GetMinerWorkerAddress(ctx context.Context, addr address.Address, tok shared.TipSetToken) (address.Address, error)

	// GetDataCap gets the current data cap for addr
	GetDataCap(ctx context.Context, addr address.Address, tok shared.TipSetToken) (*verifreg.DataCap, error)

	// GetProofType gets the current seal proof type for the given miner.
	GetProofType(ctx context.Context, addr address.Address, tok shared.TipSetToken) (abi.RegisteredSealProof, error)
}

// StorageClientNode are node dependencies for a StorageClient
type StorageClientNode interface {
	StorageCommon

	// GetStorageProviders returns information about known miners
	ListStorageProviders(ctx context.Context, tok shared.TipSetToken) ([]*StorageProviderInfo, error)

	// ValidatePublishedDeal verifies a deal is published on chain and returns the dealID
	ValidatePublishedDeal(ctx context.Context, deal ClientDeal) (abi.DealID, error)

	// SignProposal signs a DealProposal
	SignProposal(ctx context.Context, signer address.Address, proposal market.DealProposal) (*market.ClientDealProposal, error)

	// GetDefaultWalletAddress returns the address for this client
	GetDefaultWalletAddress(ctx context.Context) (address.Address, error)

	// GetMinerInfo returns info for a single miner with the given address
	GetMinerInfo(ctx context.Context, maddr address.Address, tok shared.TipSetToken) (*StorageProviderInfo, error)
}
