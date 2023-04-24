package retrievalmarket

import (
	"context"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/shared"
)

type PayloadCID = cid.Cid

// BlockstoreAccessor is used by the retrieval market client to get a
// blockstore when needed, concretely to store blocks received from the provider.
// This abstraction allows the caller to provider any blockstore implementation:
// a CARv2 file, an IPFS blockstore, or something else.
type BlockstoreAccessor interface {
	Get(DealID, PayloadCID) (bstore.Blockstore, error)
	Done(DealID) error
}

// ClientSubscriber is a callback that is registered to listen for retrieval events
type ClientSubscriber func(event ClientEvent, state ClientDealState)

type RetrieveResponse struct {
	DealID      DealID
	CarFilePath string
}

// RetrievalClient is a client interface for making retrieval deals
type RetrievalClient interface {

	// NextID generates a new deal ID.
	NextID() DealID

	// Start initializes the client by running migrations
	Start(ctx context.Context) error

	// OnReady registers a listener for when the client comes on line
	OnReady(shared.ReadyFunc)

	// Find Providers finds retrieval providers who may be storing a given piece
	FindProviders(payloadCID cid.Cid) []RetrievalPeer

	// Query asks a provider for information about a piece it is storing
	Query(
		ctx context.Context,
		p RetrievalPeer,
		payloadCID cid.Cid,
		params QueryParams,
	) (QueryResponse, error)

	// Retrieve retrieves all or part of a piece with the given retrieval parameters
	Retrieve(
		ctx context.Context,
		id DealID,
		payloadCID cid.Cid,
		params Params,
		totalFunds abi.TokenAmount,
		p RetrievalPeer,
		clientWallet address.Address,
		minerWallet address.Address,
	) (DealID, error)

	// SubscribeToEvents listens for events that happen related to client retrievals
	SubscribeToEvents(subscriber ClientSubscriber) Unsubscribe

	// V1

	// TryRestartInsufficientFunds attempts to restart any deals stuck in the insufficient funds state
	// after funds are added to a given payment channel
	TryRestartInsufficientFunds(paymentChannel address.Address) error

	// CancelDeal attempts to cancel an inprogress deal
	CancelDeal(id DealID) error

	// GetDeal returns a given deal by deal ID, if it exists
	GetDeal(dealID DealID) (ClientDealState, error)

	// ListDeals returns all deals
	ListDeals() (map[DealID]ClientDealState, error)
}
