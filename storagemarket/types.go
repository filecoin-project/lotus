package storagemarket

import (
	"context"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
)

const DealProtocolID = "/fil/storage/mk/1.0.1"
const AskProtocolID = "/fil/storage/ask/1.0.1"

// type shims - used during migration into separate module
type Balance = actors.StorageParticipantBalance
type BigInt = types.BigInt
type Signature = types.Signature
type StorageDeal = actors.OnChainDeal
type StorageAsk = types.SignedStorageAsk

// Duplicated from deals package for now
type MinerDeal struct {
	Client      peer.ID
	Proposal    actors.StorageDealProposal
	ProposalCid cid.Cid
	State       api.DealState

	Ref cid.Cid

	DealID   uint64
	SectorID uint64 // Set when State >= DealStaged
}

// The interface provided for storage providers
type StorageProvider interface {
	Run(ctx context.Context, host host.Host)

	Stop()

	AddAsk(price BigInt, ttlsecs int64) error

	// ListAsks lists current asks
	ListAsks(addr address.Address) []*StorageAsk

	// ListDeals lists on-chain deals associated with this provider
	ListDeals(ctx context.Context) ([]StorageDeal, error)

	// ListIncompleteDeals lists deals that are in progress or rejected
	ListIncompleteDeals() ([]MinerDeal, error)

	// AddStorageCollateral adds storage collateral
	AddStorageCollateral(ctx context.Context, amount BigInt) error

	// GetStorageCollateral returns the current collateral balance
	GetStorageCollateral(ctx context.Context) (Balance, error)
}
