package retrievalmarket

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/shared"
)

// ProviderSubscriber is a callback that is registered to listen for retrieval events on a provider
type ProviderSubscriber func(event ProviderEvent, state ProviderDealState)

// ProviderQueryEventSubscriber is a callback that is registered to listen for query message events
type ProviderQueryEventSubscriber func(evt ProviderQueryEvent)

// ProviderValidationSubscriber is a callback that is registered to listen for validation events
type ProviderValidationSubscriber func(evt ProviderValidationEvent)

// RetrievalProvider is an interface by which a provider configures their
// retrieval operations and monitors deals received and process
type RetrievalProvider interface {
	// Start begins listening for deals on the given host
	Start(ctx context.Context) error

	// OnReady registers a listener for when the provider comes on line
	OnReady(shared.ReadyFunc)

	// Stop stops handling incoming requests
	Stop() error

	// SetAsk sets the retrieval payment parameters that this miner will accept
	SetAsk(ask *Ask)

	// GetAsk returns the retrieval providers pricing information
	GetAsk() *Ask

	// GetDynamicAsk quotes a dynamic price for the retrieval deal by calling the user configured
	// dynamic pricing function. It passes the static price parameters set in the Ask Store to the pricing function.
	GetDynamicAsk(ctx context.Context, input PricingInput, storageDeals []abi.DealID) (Ask, error)

	// SubscribeToEvents listens for events that happen related to client retrievals
	SubscribeToEvents(subscriber ProviderSubscriber) Unsubscribe

	// SubscribeToQueryEvents subscribes to an event that is fired when a message
	// is received on the query protocol
	SubscribeToQueryEvents(subscriber ProviderQueryEventSubscriber) Unsubscribe

	// SubscribeToValidationEvents subscribes to an event that is fired when the
	// provider validates a request for data
	SubscribeToValidationEvents(subscriber ProviderValidationSubscriber) Unsubscribe

	ListDeals() map[ProviderDealIdentifier]ProviderDealState
}

// AskStore is an interface which provides access to a persisted retrieval Ask
type AskStore interface {
	GetAsk() *Ask
	SetAsk(ask *Ask) error
}
