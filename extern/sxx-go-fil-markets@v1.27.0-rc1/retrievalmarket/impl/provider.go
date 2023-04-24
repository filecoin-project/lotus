package retrievalimpl

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hannahhoward/go-pubsub"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	versionedfsm "github.com/filecoin-project/go-ds-versioning/pkg/fsm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/askstore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/stores"
)

// MaxIdentityCIDBytes is the largest identity CID as a PayloadCID that we are
// willing to decode
const MaxIdentityCIDBytes = 2 << 10

// MaxIdentityCIDLinks is the maximum number of links contained within an
// identity CID that we are willing to check for matching pieces
const MaxIdentityCIDLinks = 32

// RetrievalProviderOption is a function that configures a retrieval provider
type RetrievalProviderOption func(p *Provider)

// DealDecider is a function that makes a decision about whether to accept a deal
type DealDecider func(ctx context.Context, state retrievalmarket.ProviderDealState) (bool, string, error)

type RetrievalPricingFunc func(ctx context.Context, dealPricingParams retrievalmarket.PricingInput) (retrievalmarket.Ask, error)

var queryTimeout = 5 * time.Second

// Provider is the production implementation of the RetrievalProvider interface
type Provider struct {
	dataTransfer         datatransfer.Manager
	node                 retrievalmarket.RetrievalProviderNode
	sa                   retrievalmarket.SectorAccessor
	network              rmnet.RetrievalMarketNetwork
	requestValidator     *requestvalidation.ProviderRequestValidator
	minerAddress         address.Address
	pieceStore           piecestore.PieceStore
	readyMgr             *shared.ReadyManager
	subscribers          *pubsub.PubSub
	subQueryEvt          *pubsub.PubSub
	stateMachines        fsm.Group
	migrateStateMachines func(context.Context) error
	dealDecider          DealDecider
	askStore             retrievalmarket.AskStore
	disableNewDeals      bool
	retrievalPricingFunc RetrievalPricingFunc
	dagStore             stores.DAGStoreWrapper
	stores               *stores.ReadOnlyBlockstores
}

type internalProviderEvent struct {
	evt   retrievalmarket.ProviderEvent
	state retrievalmarket.ProviderDealState
}

func providerDispatcher(evt pubsub.Event, subscriberFn pubsub.SubscriberFn) error {
	ie, ok := evt.(internalProviderEvent)
	if !ok {
		return errors.New("wrong type of event")
	}
	cb, ok := subscriberFn.(retrievalmarket.ProviderSubscriber)
	if !ok {
		return errors.New("wrong type of event")
	}
	log.Debugw("process retrieval provider] listeners", "name", retrievalmarket.ProviderEvents[ie.evt], "proposal cid", ie.state.ID)
	cb(ie.evt, ie.state)
	return nil
}

var _ retrievalmarket.RetrievalProvider = new(Provider)

// DealDeciderOpt sets a custom protocol
func DealDeciderOpt(dd DealDecider) RetrievalProviderOption {
	return func(provider *Provider) {
		provider.dealDecider = dd
	}
}

// NewProvider returns a new retrieval Provider
func NewProvider(minerAddress address.Address,
	node retrievalmarket.RetrievalProviderNode,
	sa retrievalmarket.SectorAccessor,
	network rmnet.RetrievalMarketNetwork,
	pieceStore piecestore.PieceStore,
	dagStore stores.DAGStoreWrapper,
	dataTransfer datatransfer.Manager,
	ds datastore.Batching,
	retrievalPricingFunc RetrievalPricingFunc,
	opts ...RetrievalProviderOption,
) (retrievalmarket.RetrievalProvider, error) {

	if retrievalPricingFunc == nil {
		return nil, xerrors.New("retrievalPricingFunc is nil")
	}

	p := &Provider{
		dataTransfer:         dataTransfer,
		node:                 node,
		sa:                   sa,
		network:              network,
		minerAddress:         minerAddress,
		pieceStore:           pieceStore,
		subscribers:          pubsub.New(providerDispatcher),
		subQueryEvt:          pubsub.New(queryEvtDispatcher),
		readyMgr:             shared.NewReadyManager(),
		retrievalPricingFunc: retrievalPricingFunc,
		dagStore:             dagStore,
		stores:               stores.NewReadOnlyBlockstores(),
	}

	askStore, err := askstore.NewAskStore(namespace.Wrap(ds, datastore.NewKey("retrieval-ask")), datastore.NewKey("latest"))
	if err != nil {
		return nil, err
	}
	p.askStore = askStore

	retrievalMigrations, err := migrations.ProviderMigrations.Build()
	if err != nil {
		return nil, err
	}
	p.stateMachines, p.migrateStateMachines, err = versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
		Environment:     &providerDealEnvironment{p},
		StateType:       retrievalmarket.ProviderDealState{},
		StateKeyField:   "Status",
		Events:          providerstates.ProviderEvents,
		StateEntryFuncs: providerstates.ProviderStateEntryFuncs,
		FinalityStates:  providerstates.ProviderFinalityStates,
		Notifier:        p.notifySubscribers,
		Options: fsm.Options{
			ConsumeAllEventsBeforeEntryFuncs: true,
		},
	}, retrievalMigrations, "2")
	if err != nil {
		return nil, err
	}
	p.Configure(opts...)
	p.requestValidator = requestvalidation.NewProviderRequestValidator(&providerValidationEnvironment{p})
	transportConfigurer := dtutils.TransportConfigurer(network.ID(), &providerStoreGetter{p})

	err = p.dataTransfer.RegisterVoucherType(retrievalmarket.DealProposalType, p.requestValidator)
	if err != nil {
		return nil, err
	}

	err = p.dataTransfer.RegisterVoucherType(retrievalmarket.DealPaymentType, p.requestValidator)
	if err != nil {
		return nil, err
	}

	err = p.dataTransfer.RegisterTransportConfigurer(retrievalmarket.DealProposalType, transportConfigurer)
	if err != nil {
		return nil, err
	}

	dataTransfer.SubscribeToEvents(dtutils.ProviderDataTransferSubscriber(p.stateMachines))
	return p, nil
}

// Stop stops handling incoming requests.
func (p *Provider) Stop() error {
	return p.network.StopHandlingRequests()
}

// Start begins listening for deals on the given host.
// Start must be called in order to accept incoming deals.
func (p *Provider) Start(ctx context.Context) error {
	go func() {
		err := p.migrateStateMachines(ctx)
		if err != nil {
			log.Errorf("Migrating retrieval provider state machines: %s", err.Error())
		}
		err = p.readyMgr.FireReady(err)
		if err != nil {
			log.Warnf("Publish retrieval provider ready event: %s", err.Error())
		}
	}()
	return p.network.SetDelegate(p)
}

// OnReady registers a listener for when the provider has finished starting up
func (p *Provider) OnReady(ready shared.ReadyFunc) {
	p.readyMgr.OnReady(ready)
}

func (p *Provider) notifySubscribers(eventName fsm.EventName, state fsm.StateType) {
	evt := eventName.(retrievalmarket.ProviderEvent)
	ds := state.(retrievalmarket.ProviderDealState)
	_ = p.subscribers.Publish(internalProviderEvent{evt, ds})
}

// SubscribeToEvents listens for events that happen related to client retrievals
func (p *Provider) SubscribeToEvents(subscriber retrievalmarket.ProviderSubscriber) retrievalmarket.Unsubscribe {
	return retrievalmarket.Unsubscribe(p.subscribers.Subscribe(subscriber))
}

// SubscribeToValidationEvents subscribes to an event that is fired when the
// provider validates a request for data
func (p *Provider) SubscribeToValidationEvents(subscriber retrievalmarket.ProviderValidationSubscriber) retrievalmarket.Unsubscribe {
	return p.requestValidator.Subscribe(subscriber)
}

// GetAsk returns the current deal parameters this provider accepts
func (p *Provider) GetAsk() *retrievalmarket.Ask {
	return p.askStore.GetAsk()
}

// SetAsk sets the deal parameters this provider accepts
func (p *Provider) SetAsk(ask *retrievalmarket.Ask) {

	err := p.askStore.SetAsk(ask)

	if err != nil {
		log.Warnf("Error setting retrieval ask: %w", err)
	}
}

// ListDeals lists all known retrieval deals
func (p *Provider) ListDeals() map[retrievalmarket.ProviderDealIdentifier]retrievalmarket.ProviderDealState {
	var deals []retrievalmarket.ProviderDealState
	_ = p.stateMachines.List(&deals)
	dealMap := make(map[retrievalmarket.ProviderDealIdentifier]retrievalmarket.ProviderDealState)
	for _, deal := range deals {
		dealMap[retrievalmarket.ProviderDealIdentifier{Receiver: deal.Receiver, DealID: deal.ID}] = deal
	}
	return dealMap
}

// SubscribeToQueryEvents subscribes to an event that is fired when a message
// is received on the query protocol
func (p *Provider) SubscribeToQueryEvents(subscriber retrievalmarket.ProviderQueryEventSubscriber) retrievalmarket.Unsubscribe {
	return retrievalmarket.Unsubscribe(p.subQueryEvt.Subscribe(subscriber))
}

func queryEvtDispatcher(evt pubsub.Event, subscriberFn pubsub.SubscriberFn) error {
	e, ok := evt.(retrievalmarket.ProviderQueryEvent)
	if !ok {
		return errors.New("wrong type of event")
	}
	cb, ok := subscriberFn.(retrievalmarket.ProviderQueryEventSubscriber)
	if !ok {
		return errors.New("wrong type of callback")
	}
	cb(e)
	return nil
}

/*
HandleQueryStream is called by the network implementation whenever a new message is received on the query protocol

A Provider handling a retrieval `Query` does the following:

1. Get the node's chain head in order to get its miner worker address.

2. Look in its piece store to determine if it can serve the given payload CID.

3. Combine these results with its existing parameters for retrieval deals to construct a `retrievalmarket.QueryResponse` struct.

4. Writes this response to the `Query` stream.

The connection is kept open only as long as the query-response exchange.
*/
func (p *Provider) HandleQueryStream(stream rmnet.RetrievalQueryStream) {
	ctx, cancel := context.WithTimeout(context.TODO(), queryTimeout)
	defer cancel()

	defer stream.Close()
	query, err := stream.ReadQuery()
	if err != nil {
		return
	}

	sendResp := func(resp retrievalmarket.QueryResponse) {
		msgEvt := retrievalmarket.ProviderQueryEvent{
			Response: resp,
		}
		if err := stream.WriteQueryResponse(resp); err != nil {
			err = fmt.Errorf("Retrieval query: writing query response: %w", err)
			log.Error(err)
			msgEvt.Error = err
		}
		p.subQueryEvt.Publish(msgEvt)
	}

	answer := retrievalmarket.QueryResponse{
		Status:          retrievalmarket.QueryResponseUnavailable,
		PieceCIDFound:   retrievalmarket.QueryItemUnavailable,
		MinPricePerByte: big.Zero(),
		UnsealPrice:     big.Zero(),
	}

	// get chain head to query actor states.
	tok, _, err := p.node.GetChainHead(ctx)
	if err != nil {
		err = fmt.Errorf("Retrieval query: GetChainHead: %w", err)
		log.Error(err)
		p.subQueryEvt.Publish(retrievalmarket.ProviderQueryEvent{Error: err})
		return
	}

	// fetch the payment address the client should send the payment to.
	paymentAddress, err := p.node.GetMinerWorkerAddress(ctx, p.minerAddress, tok)
	if err != nil {
		log.Errorf("Retrieval query: Lookup Payment Address: %s", err)
		answer.Status = retrievalmarket.QueryResponseError
		answer.Message = fmt.Sprintf("failed to look up payment address: %s", err)
		sendResp(answer)
		return
	}
	answer.PaymentAddress = paymentAddress

	// fetch the piece from which the payload will be retrieved.
	// if user has specified the Piece in the request, we use that.
	// Otherwise, we prefer a Piece which can retrieved from an unsealed sector.
	pieceCID := cid.Undef
	if query.PieceCID != nil {
		pieceCID = *query.PieceCID
	}

	pieces, piecesErr := p.getAllPieceInfoForPayload(query.PayloadCID)
	// err may be non-nil, but we may have successfuly found >0 pieces, so defer error handling till
	// we have no other option.

	pieceInfo, isUnsealed := p.getBestPieceInfoMatch(ctx, pieces, pieceCID)
	if !pieceInfo.Defined() {
		if piecesErr != nil {
			log.Errorf("Retrieval query: getPieceInfoFromCid: %s", piecesErr)
			if !errors.Is(piecesErr, retrievalmarket.ErrNotFound) {
				answer.Status = retrievalmarket.QueryResponseError
				answer.Message = fmt.Sprintf("failed to fetch piece to retrieve from: %s", piecesErr)
			}
		}
		if answer.Message == "" {
			answer.Message = "piece info for cid not found (deal has not been added to a piece yet)"
		}
		sendResp(answer)
		return
	}

	answer.Status = retrievalmarket.QueryResponseAvailable
	answer.Size = uint64(pieceInfo.Deals[0].Length.Unpadded()) // TODO: verify on intermediate
	answer.PieceCIDFound = retrievalmarket.QueryItemAvailable

	storageDeals := p.getStorageDealsForPiece(query.PieceCID != nil, pieces, pieceInfo)

	if len(storageDeals) == 0 {
		log.Errorf("Retrieval query: storageDealsForPiece: %s", err)
		answer.Status = retrievalmarket.QueryResponseError
		if piecesErr != nil {
			answer.Message = fmt.Sprintf("failed to fetch storage deals containing payload [%s]: %s", query.PayloadCID.String(), piecesErr.Error())
		} else {
			answer.Message = fmt.Sprintf("failed to fetch storage deals containing payload [%s]", query.PayloadCID.String())
		}
		sendResp(answer)
		return
	}

	input := retrievalmarket.PricingInput{
		// piece from which the payload will be retrieved
		// If user hasn't given a PieceCID, we try to choose an unsealed piece in the call to `getPieceInfoFromCid` above.
		PieceCID: pieceInfo.PieceCID,

		PayloadCID: query.PayloadCID,
		Unsealed:   isUnsealed,
		Client:     stream.RemotePeer(),
	}
	ask, err := p.GetDynamicAsk(ctx, input, storageDeals)
	if err != nil {
		log.Errorf("Retrieval query: GetAsk: %s", err)
		answer.Status = retrievalmarket.QueryResponseError
		answer.Message = fmt.Sprintf("failed to price deal: %s", err)
		sendResp(answer)
		return
	}

	answer.MinPricePerByte = ask.PricePerByte
	answer.MaxPaymentInterval = ask.PaymentInterval
	answer.MaxPaymentIntervalIncrease = ask.PaymentIntervalIncrease
	answer.UnsealPrice = ask.UnsealPrice
	sendResp(answer)
}

// GetDynamicAsk quotes a dynamic price for the retrieval deal by calling the user configured
// dynamic pricing function. It passes the static price parameters set in the Ask Store to the pricing function.
func (p *Provider) GetDynamicAsk(ctx context.Context, input retrievalmarket.PricingInput, storageDeals []abi.DealID) (retrievalmarket.Ask, error) {
	dp, err := p.node.GetRetrievalPricingInput(ctx, input.PieceCID, storageDeals)
	if err != nil {
		return retrievalmarket.Ask{}, xerrors.Errorf("GetRetrievalPricingInput: %s", err)
	}
	// currAsk cannot be nil as we initialize the ask store with a default ask.
	// Users can then change the values in the ask store using SetAsk but not remove it.
	currAsk := p.GetAsk()
	if currAsk == nil {
		return retrievalmarket.Ask{}, xerrors.New("no ask configured in ask-store")
	}

	dp.PayloadCID = input.PayloadCID
	dp.PieceCID = input.PieceCID
	dp.Unsealed = input.Unsealed
	dp.Client = input.Client
	dp.CurrentAsk = *currAsk

	ask, err := p.retrievalPricingFunc(ctx, dp)
	if err != nil {
		return retrievalmarket.Ask{}, xerrors.Errorf("retrievalPricingFunc: %w", err)
	}
	return ask, nil
}

// Configure reconfigures a provider after initialization
func (p *Provider) Configure(opts ...RetrievalProviderOption) {
	for _, opt := range opts {
		opt(p)
	}
}

// ProviderFSMParameterSpec is a valid set of parameters for a provider FSM - used in doc generation
var ProviderFSMParameterSpec = fsm.Parameters{
	Environment:     &providerDealEnvironment{},
	StateType:       retrievalmarket.ProviderDealState{},
	StateKeyField:   "Status",
	Events:          providerstates.ProviderEvents,
	StateEntryFuncs: providerstates.ProviderStateEntryFuncs,
	Options: fsm.Options{
		ConsumeAllEventsBeforeEntryFuncs: true,
	},
}

// DefaultPricingFunc is the default pricing policy that will be used to price retrieval deals.
var DefaultPricingFunc = func(VerifiedDealsFreeTransfer bool) func(ctx context.Context, pricingInput retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
	return func(ctx context.Context, pricingInput retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
		ask := pricingInput.CurrentAsk

		// don't charge for Unsealing if we have an Unsealed copy.
		if pricingInput.Unsealed {
			ask.UnsealPrice = big.Zero()
		}

		// don't charge for data transfer for verified deals if it's been configured to do so.
		if pricingInput.VerifiedDeal && VerifiedDealsFreeTransfer {
			ask.PricePerByte = big.Zero()
		}

		return ask, nil
	}
}
