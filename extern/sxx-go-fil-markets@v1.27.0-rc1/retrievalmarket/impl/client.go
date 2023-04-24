package retrievalimpl

import (
	"context"
	"errors"
	"sync"

	"github.com/hannahhoward/go-pubsub"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	versionedfsm "github.com/filecoin-project/go-ds-versioning/pkg/fsm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/discovery"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/clientstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared"
)

var log = logging.Logger("retrieval")

// Client is the production implementation of the RetrievalClient interface
type Client struct {
	network      rmnet.RetrievalMarketNetwork
	dataTransfer datatransfer.Manager
	node         retrievalmarket.RetrievalClientNode
	dealIDGen    *shared.TimeCounter

	subscribers          *pubsub.PubSub
	readySub             *pubsub.PubSub
	resolver             discovery.PeerResolver
	stateMachines        fsm.Group
	migrateStateMachines func(context.Context) error
	bstores              retrievalmarket.BlockstoreAccessor

	// Guards concurrent access to Retrieve method
	retrieveLk sync.Mutex
}

type internalEvent struct {
	evt   retrievalmarket.ClientEvent
	state retrievalmarket.ClientDealState
}

func dispatcher(evt pubsub.Event, subscriberFn pubsub.SubscriberFn) error {
	ie, ok := evt.(internalEvent)
	if !ok {
		return errors.New("wrong type of event")
	}
	cb, ok := subscriberFn.(retrievalmarket.ClientSubscriber)
	if !ok {
		return errors.New("wrong type of event")
	}
	log.Debugw("process retrieval client listeners", "name", retrievalmarket.ClientEvents[ie.evt], "proposal cid", ie.state.ID)
	cb(ie.evt, ie.state)
	return nil
}

var _ retrievalmarket.RetrievalClient = &Client{}

// NewClient creates a new retrieval client
func NewClient(
	network rmnet.RetrievalMarketNetwork,
	dataTransfer datatransfer.Manager,
	node retrievalmarket.RetrievalClientNode,
	resolver discovery.PeerResolver,
	ds datastore.Batching,
	ba retrievalmarket.BlockstoreAccessor,
) (retrievalmarket.RetrievalClient, error) {
	c := &Client{
		network:      network,
		dataTransfer: dataTransfer,
		node:         node,
		resolver:     resolver,
		dealIDGen:    shared.NewTimeCounter(),
		subscribers:  pubsub.New(dispatcher),
		readySub:     pubsub.New(shared.ReadyDispatcher),
		bstores:      ba,
	}
	retrievalMigrations, err := migrations.ClientMigrations.Build()
	if err != nil {
		return nil, err
	}
	c.stateMachines, c.migrateStateMachines, err = versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
		Environment:     &clientDealEnvironment{c},
		StateType:       retrievalmarket.ClientDealState{},
		StateKeyField:   "Status",
		Events:          clientstates.ClientEvents,
		StateEntryFuncs: clientstates.ClientStateEntryFuncs,
		FinalityStates:  clientstates.ClientFinalityStates,
		Notifier:        c.notifySubscribers,
	}, retrievalMigrations, versioning.VersionKey("2"))
	if err != nil {
		return nil, err
	}
	err = dataTransfer.RegisterVoucherType(retrievalmarket.DealProposalType, nil)
	if err != nil {
		return nil, err
	}
	err = dataTransfer.RegisterVoucherType(retrievalmarket.DealPaymentType, nil)
	if err != nil {
		return nil, err
	}
	dataTransfer.SubscribeToEvents(dtutils.ClientDataTransferSubscriber(c.stateMachines))
	transportConfigurer := dtutils.TransportConfigurer(network.ID(), &clientStoreGetter{c})
	err = dataTransfer.RegisterTransportConfigurer(retrievalmarket.DealProposalType, transportConfigurer)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) NextID() retrievalmarket.DealID {
	return retrievalmarket.DealID(c.dealIDGen.Next())
}

// Start initialized the Client, performing relevant database migrations
func (c *Client) Start(ctx context.Context) error {
	go func() {
		err := c.migrateStateMachines(ctx)
		if err != nil {
			log.Errorf("Migrating retrieval client state machines: %s", err.Error())
		}

		err = c.readySub.Publish(err)
		if err != nil {
			log.Warnf("Publish retrieval client ready event: %s", err.Error())
		}
	}()
	return nil
}

// OnReady registers a listener for when the client has finished starting up
func (c *Client) OnReady(ready shared.ReadyFunc) {
	c.readySub.Subscribe(ready)
}

// FindProviders uses PeerResolver interface to locate a list of providers who may have a given payload CID.
func (c *Client) FindProviders(payloadCID cid.Cid) []retrievalmarket.RetrievalPeer {
	peers, err := c.resolver.GetPeers(payloadCID)
	if err != nil {
		log.Errorf("failed to get peers: %s", err)
		return []retrievalmarket.RetrievalPeer{}
	}
	return peers
}

/*
Query sends a retrieval query to a specific retrieval provider, to determine
if the provider can serve a retrieval request and what its specific parameters for
the request are.

The client creates a new `RetrievalQueryStream` for the chosen peer ID,
and calls `WriteQuery` on it, which constructs a data-transfer message and writes it to the Query stream.
*/
func (c *Client) Query(ctx context.Context, p retrievalmarket.RetrievalPeer, payloadCID cid.Cid, params retrievalmarket.QueryParams) (retrievalmarket.QueryResponse, error) {
	err := c.addMultiaddrs(ctx, p)
	if err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}
	s, err := c.network.NewQueryStream(p.ID)
	if err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}
	defer s.Close()

	err = s.WriteQuery(retrievalmarket.Query{
		PayloadCID:  payloadCID,
		QueryParams: params,
	})
	if err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	return s.ReadQueryResponse()
}

// Retrieve initiates the retrieval deal flow, which involves multiple requests and responses
//
// To start this processes, the client creates a new `RetrievalDealStream`.  Currently, this connection is
// kept open through the entire deal until completion or failure.  Make deals pauseable as well as surviving
// a restart is a planned future feature.
//
// Retrieve should be called after using FindProviders and Query are used to identify an appropriate provider to
// retrieve the deal from. The parameters identified in Query should be passed to Retrieve to ensure the
// greatest likelihood the provider will accept the deal
//
// When called, the client takes the following actions:
//
// 1. Creates a deal ID using the next value from its `storedCounter`.
//
// 2. Constructs a `DealProposal` with deal terms
//
// 3. Tells its statemachine to begin tracking this deal state by dealID.
//
// 4. Constructs a `blockio.SelectorVerifier` and adds it to its dealID-keyed map of block verifiers.
//
// 5. Triggers a `ClientEventOpen` event on its statemachine.
//
// From then on, the statemachine controls the deal flow in the client. Other components may listen for events in this flow by calling
// `SubscribeToEvents` on the Client. The Client handles consuming blocks it receives from the provider, via `ConsumeBlocks` function
//
// Retrieve can use an ID generated through NextID, or can generate an ID if the user passes a zero value.
//
// Use NextID when it's necessary to reserve an ID ahead of time, e.g. to
// associate it with a given blockstore in the BlockstoreAccessor.
//
// Documentation of the client state machine can be found at https://godoc.org/github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/clientstates
func (c *Client) Retrieve(
	ctx context.Context,
	id retrievalmarket.DealID,
	payloadCID cid.Cid,
	params retrievalmarket.Params,
	totalFunds abi.TokenAmount,
	p retrievalmarket.RetrievalPeer,
	clientWallet address.Address,
	minerWallet address.Address,
) (retrievalmarket.DealID, error) {
	c.retrieveLk.Lock()
	defer c.retrieveLk.Unlock()

	err := c.addMultiaddrs(ctx, p)
	if err != nil {
		return 0, err
	}

	// assign a new ID.
	if id == 0 {
		next := c.dealIDGen.Next()
		id = retrievalmarket.DealID(next)
	}

	dealState := retrievalmarket.ClientDealState{
		DealProposal: retrievalmarket.DealProposal{
			PayloadCID: payloadCID,
			ID:         id,
			Params:     params,
		},
		TotalFunds:       totalFunds,
		ClientWallet:     clientWallet,
		MinerWallet:      minerWallet,
		TotalReceived:    0,
		CurrentInterval:  params.PaymentInterval,
		BytesPaidFor:     0,
		PaymentRequested: abi.NewTokenAmount(0),
		FundsSpent:       abi.NewTokenAmount(0),
		Status:           retrievalmarket.DealStatusNew,
		Sender:           p.ID,
		UnsealFundsPaid:  big.Zero(),
	}

	// start the deal processing
	err = c.stateMachines.Begin(dealState.ID, &dealState)
	if err != nil {
		return 0, err
	}

	err = c.stateMachines.Send(dealState.ID, retrievalmarket.ClientEventOpen)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (c *Client) notifySubscribers(eventName fsm.EventName, state fsm.StateType) {
	evt := eventName.(retrievalmarket.ClientEvent)
	ds := state.(retrievalmarket.ClientDealState)
	_ = c.subscribers.Publish(internalEvent{evt, ds})
}

func (c *Client) addMultiaddrs(ctx context.Context, p retrievalmarket.RetrievalPeer) error {
	tok, _, err := c.node.GetChainHead(ctx)
	if err != nil {
		return err
	}
	maddrs, err := c.node.GetKnownAddresses(ctx, p, tok)
	if err != nil {
		return err
	}
	if len(maddrs) > 0 {
		c.network.AddAddrs(p.ID, maddrs)
	}
	return nil
}

// SubscribeToEvents allows another component to listen for events on the RetrievalClient
// in order to track deals as they progress through the deal flow
func (c *Client) SubscribeToEvents(subscriber retrievalmarket.ClientSubscriber) retrievalmarket.Unsubscribe {
	return retrievalmarket.Unsubscribe(c.subscribers.Subscribe(subscriber))
}

// V1

// TryRestartInsufficientFunds attempts to restart any deals stuck in the insufficient funds state
// after funds are added to a given payment channel
func (c *Client) TryRestartInsufficientFunds(paymentChannel address.Address) error {
	var deals []retrievalmarket.ClientDealState
	err := c.stateMachines.List(&deals)
	if err != nil {
		return err
	}
	for _, deal := range deals {
		if deal.Status == retrievalmarket.DealStatusInsufficientFunds && deal.PaymentInfo.PayCh == paymentChannel {
			if err := c.stateMachines.Send(deal.ID, retrievalmarket.ClientEventRecheckFunds); err != nil {
				return err
			}
		}
	}
	return nil
}

// CancelDeal attempts to cancel an in progress deal
func (c *Client) CancelDeal(dealID retrievalmarket.DealID) error {
	return c.stateMachines.Send(dealID, retrievalmarket.ClientEventCancel)
}

// GetDeal returns a given deal by deal ID, if it exists
func (c *Client) GetDeal(dealID retrievalmarket.DealID) (retrievalmarket.ClientDealState, error) {
	var out retrievalmarket.ClientDealState
	if err := c.stateMachines.Get(dealID).Get(&out); err != nil {
		return retrievalmarket.ClientDealState{}, err
	}
	return out, nil
}

// ListDeals lists all known retrieval deals
func (c *Client) ListDeals() (map[retrievalmarket.DealID]retrievalmarket.ClientDealState, error) {
	var deals []retrievalmarket.ClientDealState
	err := c.stateMachines.List(&deals)
	if err != nil {
		return nil, err
	}
	dealMap := make(map[retrievalmarket.DealID]retrievalmarket.ClientDealState)
	for _, deal := range deals {
		dealMap[deal.ID] = deal
	}
	return dealMap, nil
}

var _ clientstates.ClientDealEnvironment = &clientDealEnvironment{}

type clientDealEnvironment struct {
	c *Client
}

// Node returns the node interface for this deal
func (c *clientDealEnvironment) Node() retrievalmarket.RetrievalClientNode {
	return c.c.node
}

func (c *clientDealEnvironment) OpenDataTransfer(ctx context.Context, to peer.ID, proposal *retrievalmarket.DealProposal) (datatransfer.ChannelID, error) {
	sel := selectorparse.CommonSelector_ExploreAllRecursively
	if proposal.SelectorSpecified() {
		sel = proposal.Selector.Node
	}
	vouch := retrievalmarket.BindnodeRegistry.TypeToNode(proposal)
	return c.c.dataTransfer.OpenPullDataChannel(ctx, to, datatransfer.TypedVoucher{Voucher: vouch, Type: retrievalmarket.DealProposalType}, proposal.PayloadCID, sel)
}

func (c *clientDealEnvironment) SendDataTransferVoucher(ctx context.Context, channelID datatransfer.ChannelID, payment *retrievalmarket.DealPayment) error {
	vouch := retrievalmarket.BindnodeRegistry.TypeToNode(payment)
	return c.c.dataTransfer.SendVoucher(ctx, channelID, datatransfer.TypedVoucher{Voucher: vouch, Type: retrievalmarket.DealPaymentType})
}

func (c *clientDealEnvironment) CloseDataTransfer(ctx context.Context, channelID datatransfer.ChannelID) error {
	// When we close the data transfer, we also send a cancel message to the peer.
	// Make sure we don't wait too long to send the message.
	ctx, cancel := context.WithTimeout(ctx, shared.CloseDataTransferTimeout)
	defer cancel()

	err := c.c.dataTransfer.CloseDataTransferChannel(ctx, channelID)
	if shared.IsCtxDone(err) {
		log.Warnf("failed to send cancel data transfer channel %s to provider within timeout %s",
			channelID, shared.CloseDataTransferTimeout)
		return nil
	}
	return err
}

// FinalizeBlockstore is called when all blocks have been received
func (c *clientDealEnvironment) FinalizeBlockstore(ctx context.Context, dealID retrievalmarket.DealID) error {
	return c.c.bstores.Done(dealID)
}

type clientStoreGetter struct {
	c *Client
}

func (csg *clientStoreGetter) Get(_ peer.ID, id retrievalmarket.DealID) (bstore.Blockstore, error) {
	var deal retrievalmarket.ClientDealState
	err := csg.c.stateMachines.Get(id).Get(&deal)
	if err != nil {
		return nil, err
	}
	return csg.c.bstores.Get(id, deal.PayloadCID)
}

// ClientFSMParameterSpec is a valid set of parameters for a client deal FSM - used in doc generation
var ClientFSMParameterSpec = fsm.Parameters{
	Environment:     &clientDealEnvironment{},
	StateType:       retrievalmarket.ClientDealState{},
	StateKeyField:   "Status",
	Events:          clientstates.ClientEvents,
	StateEntryFuncs: clientstates.ClientStateEntryFuncs,
}
