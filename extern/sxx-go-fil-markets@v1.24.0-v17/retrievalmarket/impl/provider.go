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
	datatransfer "github.com/filecoin-project/go-data-transfer"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
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
	revalidator          *requestvalidation.ProviderRevalidator
	minerAddress         address.Address
	pieceStore           piecestore.PieceStore
	readyMgr             *shared.ReadyManager
	subscribers          *pubsub.PubSub
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

// DisableNewDeals disables setup for v1 deal protocols
func DisableNewDeals() RetrievalProviderOption {
	return func(provider *Provider) {
		provider.disableNewDeals = true
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
		readyMgr:             shared.NewReadyManager(),
		retrievalPricingFunc: retrievalPricingFunc,
		dagStore:             dagStore,
		stores:               stores.NewReadOnlyBlockstores(),
	}

	err := shared.MoveKey(ds, "retrieval-ask", "retrieval-ask/latest")
	if err != nil {
		return nil, err
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
	}, retrievalMigrations, versioning.VersionKey("1"))
	if err != nil {
		return nil, err
	}
	p.Configure(opts...)
	p.requestValidator = requestvalidation.NewProviderRequestValidator(&providerValidationEnvironment{p})
	transportConfigurer := dtutils.TransportConfigurer(network.ID(), &providerStoreGetter{p})
	p.revalidator = requestvalidation.NewProviderRevalidator(&providerRevalidatorEnvironment{p})

	if p.disableNewDeals {
		err = p.dataTransfer.RegisterVoucherType(&migrations.DealProposal0{}, p.requestValidator)
		if err != nil {
			return nil, err
		}
		err = p.dataTransfer.RegisterRevalidator(&migrations.DealPayment0{}, p.revalidator)
		if err != nil {
			return nil, err
		}
	} else {
		err = p.dataTransfer.RegisterVoucherType(&retrievalmarket.DealProposal{}, p.requestValidator)
		if err != nil {
			return nil, err
		}
		err = p.dataTransfer.RegisterVoucherType(&migrations.DealProposal0{}, p.requestValidator)
		if err != nil {
			return nil, err
		}

		err = p.dataTransfer.RegisterRevalidator(&retrievalmarket.DealPayment{}, p.revalidator)
		if err != nil {
			return nil, err
		}
		err = p.dataTransfer.RegisterRevalidator(&migrations.DealPayment0{}, requestvalidation.NewLegacyRevalidator(p.revalidator))
		if err != nil {
			return nil, err
		}

		err = p.dataTransfer.RegisterVoucherResultType(&retrievalmarket.DealResponse{})
		if err != nil {
			return nil, err
		}

		err = p.dataTransfer.RegisterTransportConfigurer(&retrievalmarket.DealProposal{}, transportConfigurer)
		if err != nil {
			return nil, err
		}
	}
	err = p.dataTransfer.RegisterVoucherResultType(&migrations.DealResponse0{})
	if err != nil {
		return nil, err
	}
	err = p.dataTransfer.RegisterTransportConfigurer(&migrations.DealProposal0{}, transportConfigurer)
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
		if err := stream.WriteQueryResponse(resp); err != nil {
			log.Errorf("Retrieval query: writing query response: %s", err)
		}
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
		log.Errorf("Retrieval query: GetChainHead: %s", err)
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
	pieceInfo, isUnsealed, err := p.getPieceInfoFromCid(ctx, query.PayloadCID, pieceCID)
	if err != nil {
		log.Errorf("Retrieval query: getPieceInfoFromCid: %s", err)
		if !xerrors.Is(err, retrievalmarket.ErrNotFound) {
			answer.Status = retrievalmarket.QueryResponseError
			answer.Message = fmt.Sprintf("failed to fetch piece to retrieve from: %s", err)
		} else {
			answer.Message = "piece info for cid not found (deal has not been added to a piece yet)"
		}

		sendResp(answer)
		return
	}

	answer.Status = retrievalmarket.QueryResponseAvailable
	answer.Size = uint64(pieceInfo.Deals[0].Length.Unpadded()) // TODO: verify on intermediate
	answer.PieceCIDFound = retrievalmarket.QueryItemAvailable

	storageDeals, err := p.storageDealsForPiece(query.PieceCID != nil, query.PayloadCID, pieceInfo)
	if err != nil {
		log.Errorf("Retrieval query: storageDealsForPiece: %s", err)
		answer.Status = retrievalmarket.QueryResponseError
		answer.Message = fmt.Sprintf("failed to fetch storage deals containing payload: %s", err)
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

// Given the CID of a block, find a piece that contains that block.
// If the client has specified which piece they want, return that piece.
// Otherwise prefer pieces that are already unsealed.
func (p *Provider) getPieceInfoFromCid(ctx context.Context, payloadCID, clientPieceCID cid.Cid) (piecestore.PieceInfo, bool, error) {
	// Get all pieces that contain the target block
	piecesWithTargetBlock, err := p.dagStore.GetPiecesContainingBlock(payloadCID)
	if err != nil {
		return piecestore.PieceInfoUndefined, false, xerrors.Errorf("getting pieces for cid %s: %w", payloadCID, err)
	}

	// For each piece that contains the target block
	var lastErr error
	var sealedPieceInfo *piecestore.PieceInfo
	for _, pieceWithTargetBlock := range piecesWithTargetBlock {
		// Get the deals for the piece
		pieceInfo, err := p.pieceStore.GetPieceInfo(pieceWithTargetBlock)
		if err != nil {
			lastErr = err
			continue
		}

		// if client wants to retrieve the payload from a specific piece, just return that piece.
		if clientPieceCID.Defined() && pieceInfo.PieceCID.Equals(clientPieceCID) {
			return pieceInfo, p.pieceInUnsealedSector(ctx, pieceInfo), nil
		}

		// if client doesn't have a preference for a particular piece, prefer a piece
		// for which an unsealed sector exists.
		if clientPieceCID.Equals(cid.Undef) {
			if p.pieceInUnsealedSector(ctx, pieceInfo) {
				// The piece is in an unsealed sector, so just return it
				return pieceInfo, true, nil
			}

			if sealedPieceInfo == nil {
				// The piece is not in an unsealed sector, so save it but keep
				// checking other pieces to see if there is one that is in an
				// unsealed sector
				sealedPieceInfo = &pieceInfo
			}
		}

	}

	// Found a piece containing the target block, piece is in a sealed sector
	if sealedPieceInfo != nil {
		return *sealedPieceInfo, false, nil
	}

	// Couldn't find a piece containing the target block
	if lastErr == nil {
		lastErr = xerrors.Errorf("unknown pieceCID %s", clientPieceCID.String())
	}

	// Error finding a piece containing the target block
	return piecestore.PieceInfoUndefined, false, xerrors.Errorf("could not locate piece: %w", lastErr)
}

func (p *Provider) pieceInUnsealedSector(ctx context.Context, pieceInfo piecestore.PieceInfo) bool {
	for _, di := range pieceInfo.Deals {
		isUnsealed, err := p.sa.IsUnsealed(ctx, di.SectorID, di.Offset.Unpadded(), di.Length.Unpadded())
		if err != nil {
			log.Errorf("failed to find out if sector %d is unsealed, err=%s", di.SectorID, err)
			continue
		}
		if isUnsealed {
			return true
		}
	}

	return false
}

func (p *Provider) storageDealsForPiece(clientSpecificPiece bool, payloadCID cid.Cid, pieceInfo piecestore.PieceInfo) ([]abi.DealID, error) {
	var storageDeals []abi.DealID
	var err error
	if clientSpecificPiece {
		// If the user wants to retrieve the payload from a specific piece,
		// we only need to inspect storage deals made for that piece to quote a price.
		for _, d := range pieceInfo.Deals {
			storageDeals = append(storageDeals, d.DealID)
		}
	} else {
		// If the user does NOT want to retrieve from a specific piece, we'll have to inspect all storage deals
		// made for that piece to quote a price.
		storageDeals, err = p.getAllDealsContainingPayload(payloadCID)
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch deals for payload: %w", err)
		}
	}

	if len(storageDeals) == 0 {
		return nil, xerrors.New("no storage deals found")
	}

	return storageDeals, nil
}

func (p *Provider) getAllDealsContainingPayload(payloadCID cid.Cid) ([]abi.DealID, error) {
	// Get all pieces that contain the target block
	piecesWithTargetBlock, err := p.dagStore.GetPiecesContainingBlock(payloadCID)
	if err != nil {
		return nil, xerrors.Errorf("getting pieces for cid %s: %w", payloadCID, err)
	}

	// For each piece that contains the target block
	var lastErr error
	var dealsIds []abi.DealID
	for _, pieceWithTargetBlock := range piecesWithTargetBlock {
		// Get the deals for the piece
		pieceInfo, err := p.pieceStore.GetPieceInfo(pieceWithTargetBlock)
		if err != nil {
			lastErr = err
			continue
		}

		for _, d := range pieceInfo.Deals {
			dealsIds = append(dealsIds, d.DealID)
		}
	}

	if lastErr == nil && len(dealsIds) == 0 {
		return nil, xerrors.New("no deals found")
	}

	if lastErr != nil && len(dealsIds) == 0 {
		return nil, xerrors.Errorf("failed to fetch deals containing payload %s: %w", payloadCID, lastErr)
	}

	return dealsIds, nil
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
