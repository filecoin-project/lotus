package retrievalmarket

import (
	"context"

	"github.com/filecoin-project/go-fil-markets/shared"
)

// ProviderSubscriber is a callback that is registered to listen for retrieval events on a provider
type ProviderSubscriber func(event ProviderEvent, state ProviderDealState)

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

	// SubscribeToEvents listens for events that happen related to client retrievals
	SubscribeToEvents(subscriber ProviderSubscriber) Unsubscribe

	ListDeals() map[ProviderDealIdentifier]ProviderDealState
}

// AskStore is an interface which provides access to a persisted retrieval Ask
type AskStore interface {
	GetAsk() *Ask
	SetAsk(ask *Ask) error
}
