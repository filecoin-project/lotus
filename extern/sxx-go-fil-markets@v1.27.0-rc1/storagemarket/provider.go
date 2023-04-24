package storagemarket

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/shared"
)

// ProviderSubscriber is a callback that is run when events are emitted on a StorageProvider
type ProviderSubscriber func(event ProviderEvent, deal MinerDeal)

// StorageProvider provides an interface to the storage market for a single
// storage miner.
type StorageProvider interface {

	// Start initializes deal processing on a StorageProvider and restarts in progress deals.
	// It also registers the provider with a StorageMarketNetwork so it can receive incoming
	// messages on the storage market's libp2p protocols
	Start(ctx context.Context) error

	// OnReady registers a listener for when the provider comes on line
	OnReady(shared.ReadyFunc)

	// Stop terminates processing of deals on a StorageProvider
	Stop() error

	// SetAsk configures the storage miner's ask with the provided prices (for unverified and verified deals),
	// duration, and options. Any previously-existing ask is replaced.
	SetAsk(price abi.TokenAmount, verifiedPrice abi.TokenAmount, duration abi.ChainEpoch, options ...StorageAskOption) error

	// GetAsk returns the storage miner's ask, or nil if one does not exist.
	GetAsk() *SignedStorageAsk

	// GetLocalDeal gets a deal by signed proposal cid
	GetLocalDeal(cid cid.Cid) (MinerDeal, error)

	// LocalDealCount gets the number of local deals
	LocalDealCount() (int, error)

	// ListLocalDeals lists deals processed by this storage provider
	ListLocalDeals() ([]MinerDeal, error)

	// ListLocalDealsPage lists deals by creation time descending, starting
	// at the deal with the given signed proposal cid, skipping offset deals
	// and returning up to limit deals
	ListLocalDealsPage(startPropCid *cid.Cid, offset int, limit int) ([]MinerDeal, error)

	// AddStorageCollateral adds storage collateral
	AddStorageCollateral(ctx context.Context, amount abi.TokenAmount) error

	// GetStorageCollateral returns the current collateral balance
	GetStorageCollateral(ctx context.Context) (Balance, error)

	// ImportDataForDeal manually imports data for an offline storage deal
	ImportDataForDeal(ctx context.Context, propCid cid.Cid, data io.Reader) error

	// add by lin
	ImportDataForDealOfSxx(ctx context.Context, propCid cid.Cid, path string) error
	// end

	// SubscribeToEvents listens for events that happen related to storage deals on a provider
	SubscribeToEvents(subscriber ProviderSubscriber) shared.Unsubscribe

	RetryDealPublishing(propCid cid.Cid) error

	AnnounceDealToIndexer(ctx context.Context, proposalCid cid.Cid) error

	AnnounceAllDealsToIndexer(ctx context.Context) error
}
