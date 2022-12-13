package storageimpl

import (
	"context"
	"fmt"
	"time"

	"github.com/hannahhoward/go-pubsub"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	versionedfsm "github.com/filecoin-project/go-ds-versioning/pkg/fsm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-statemachine/fsm"

	discoveryimpl "github.com/filecoin-project/go-fil-markets/discovery/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

var log = logging.Logger("storagemarket_impl")

// DefaultPollingInterval is the frequency with which we query the provider for a status update
const DefaultPollingInterval = 30 * time.Second

// DefaultMaxTraversalLinks is the maximum number of links to traverse during CommP calculation
// before returning an error
const DefaultMaxTraversalLinks = 2 << 29

var _ storagemarket.StorageClient = &Client{}

// Client is the production implementation of the StorageClient interface
type Client struct {
	net network.StorageMarketNetwork

	dataTransfer         datatransfer.Manager
	discovery            *discoveryimpl.Local
	node                 storagemarket.StorageClientNode
	pubSub               *pubsub.PubSub
	readySub             *pubsub.PubSub
	statemachines        fsm.Group
	migrateStateMachines func(context.Context) error
	pollingInterval      time.Duration
	maxTraversalLinks    uint64

	unsubDataTransfer datatransfer.Unsubscribe

	bstores storagemarket.BlockstoreAccessor
}

// StorageClientOption allows custom configuration of a storage client
type StorageClientOption func(c *Client)

// DealPollingInterval sets the interval at which this client will query the Provider for deal state while
// waiting for deal acceptance
func DealPollingInterval(t time.Duration) StorageClientOption {
	return func(c *Client) {
		c.pollingInterval = t
	}
}

// MaxTraversalLinks sets the maximum number of links in a DAG to traverse when calculating CommP,
// sets a budget that limits the depth and density of a DAG that can be traversed
func MaxTraversalLinks(m uint64) StorageClientOption {
	return func(c *Client) {
		c.maxTraversalLinks = m
	}
}

// NewClient creates a new storage client
func NewClient(
	net network.StorageMarketNetwork,
	dataTransfer datatransfer.Manager,
	discovery *discoveryimpl.Local,
	ds datastore.Batching,
	scn storagemarket.StorageClientNode,
	bstores storagemarket.BlockstoreAccessor,
	options ...StorageClientOption,
) (*Client, error) {
	c := &Client{
		net:               net,
		dataTransfer:      dataTransfer,
		discovery:         discovery,
		node:              scn,
		pubSub:            pubsub.New(clientDispatcher),
		readySub:          pubsub.New(shared.ReadyDispatcher),
		pollingInterval:   DefaultPollingInterval,
		maxTraversalLinks: DefaultMaxTraversalLinks,
		bstores:           bstores,
	}
	storageMigrations, err := migrations.ClientMigrations.Build()
	if err != nil {
		return nil, err
	}
	c.statemachines, c.migrateStateMachines, err = newClientStateMachine(
		ds,
		&clientDealEnvironment{c},
		c.dispatch,
		storageMigrations,
		versioning.VersionKey("1"),
	)
	if err != nil {
		return nil, err
	}

	c.Configure(options...)

	// register a data transfer event handler -- this will send events to the state machines based on DT events
	c.unsubDataTransfer = dataTransfer.SubscribeToEvents(dtutils.ClientDataTransferSubscriber(c.statemachines))

	err = dataTransfer.RegisterVoucherType(&requestvalidation.StorageDataTransferVoucher{}, requestvalidation.NewUnifiedRequestValidator(nil, &clientPullDeals{c}))
	if err != nil {
		return nil, err
	}

	err = dataTransfer.RegisterTransportConfigurer(&requestvalidation.StorageDataTransferVoucher{}, dtutils.TransportConfigurer(&clientStoreGetter{c}))
	if err != nil {
		return nil, err
	}

	return c, nil
}

// Start initializes deal processing on a StorageClient, runs migrations and restarts
// in progress deals
func (c *Client) Start(ctx context.Context) error {
	go func() {
		err := c.start(ctx)
		if err != nil {
			log.Error(err.Error())
		}
	}()
	return nil
}

// OnReady registers a listener for when the client has finished starting up
func (c *Client) OnReady(ready shared.ReadyFunc) {
	c.readySub.Subscribe(ready)
}

// Stop ends deal processing on a StorageClient
func (c *Client) Stop() error {
	c.unsubDataTransfer()
	return c.statemachines.Stop(context.TODO())
}

// ListProviders queries chain state and returns active storage providers
func (c *Client) ListProviders(ctx context.Context) (<-chan storagemarket.StorageProviderInfo, error) {
	tok, _, err := c.node.GetChainHead(ctx)
	if err != nil {
		return nil, err
	}

	providers, err := c.node.ListStorageProviders(ctx, tok)
	if err != nil {
		return nil, err
	}

	out := make(chan storagemarket.StorageProviderInfo)

	go func() {
		defer close(out)
		for _, p := range providers {
			select {
			case out <- *p:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// ListLocalDeals lists deals initiated by this storage client
func (c *Client) ListLocalDeals(ctx context.Context) ([]storagemarket.ClientDeal, error) {
	var out []storagemarket.ClientDeal
	if err := c.statemachines.List(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetLocalDeal lists deals that are in progress or rejected
func (c *Client) GetLocalDeal(ctx context.Context, cid cid.Cid) (storagemarket.ClientDeal, error) {
	var out storagemarket.ClientDeal
	if err := c.statemachines.Get(cid).Get(&out); err != nil {
		return storagemarket.ClientDeal{}, err
	}
	return out, nil
}

// GetAsk queries a provider for its current storage ask
//
// The client creates a new `StorageAskStream` for the chosen peer ID,
// and calls WriteAskRequest on it, which constructs a message and writes it to the Ask stream.
// When it receives a response, it verifies the signature and returns the validated
// StorageAsk if successful
func (c *Client) GetAsk(ctx context.Context, info storagemarket.StorageProviderInfo) (*storagemarket.StorageAsk, error) {
	if len(info.Addrs) > 0 {
		c.net.AddAddrs(info.PeerID, info.Addrs)
	}
	s, err := c.net.NewAskStream(ctx, info.PeerID)
	if err != nil {
		return nil, xerrors.Errorf("failed to open stream to miner: %w", err)
	}
	defer s.Close() //nolint

	request := network.AskRequest{Miner: info.Address}
	if err := s.WriteAskRequest(request); err != nil {
		return nil, xerrors.Errorf("failed to send ask request: %w", err)
	}

	out, origBytes, err := s.ReadAskResponse()
	if err != nil {
		return nil, xerrors.Errorf("failed to read ask response: %w", err)
	}

	if out.Ask == nil {
		return nil, xerrors.Errorf("got no ask back")
	}

	if out.Ask.Ask.Miner != info.Address {
		return nil, xerrors.Errorf("got back ask for wrong miner")
	}

	tok, _, err := c.node.GetChainHead(ctx)
	if err != nil {
		return nil, err
	}

	isValid, err := c.node.VerifySignature(ctx, *out.Ask.Signature, info.Worker, origBytes, tok)
	if err != nil {
		return nil, err
	}

	if !isValid {
		return nil, xerrors.Errorf("ask was not properly signed")
	}

	return out.Ask.Ask, nil
}

// GetProviderDealState queries a provider for the current state of a client's deal
func (c *Client) GetProviderDealState(ctx context.Context, proposalCid cid.Cid) (*storagemarket.ProviderDealState, error) {
	var deal storagemarket.ClientDeal
	err := c.statemachines.Get(proposalCid).Get(&deal)
	if err != nil {
		return nil, xerrors.Errorf("could not get client deal state: %w", err)
	}

	s, err := c.net.NewDealStatusStream(ctx, deal.Miner)
	if err != nil {
		return nil, xerrors.Errorf("failed to open stream to miner: %w", err)
	}
	defer s.Close() //nolint

	buf, err := cborutil.Dump(&deal.ProposalCid)
	if err != nil {
		return nil, xerrors.Errorf("failed serialize deal status request: %w", err)
	}

	signature, err := c.node.SignBytes(ctx, deal.Proposal.Client, buf)
	if err != nil {
		return nil, xerrors.Errorf("failed to sign deal status request: %w", err)
	}

	if err := s.WriteDealStatusRequest(network.DealStatusRequest{Proposal: proposalCid, Signature: *signature}); err != nil {
		return nil, xerrors.Errorf("failed to send deal status request: %w", err)
	}

	resp, origBytes, err := s.ReadDealStatusResponse()
	if err != nil {
		return nil, xerrors.Errorf("failed to read deal status response: %w", err)
	}

	valid, err := c.verifyStatusResponseSignature(ctx, deal.MinerWorker, resp, origBytes)
	if err != nil {
		return nil, err
	}

	if !valid {
		return nil, xerrors.Errorf("invalid deal status response signature")
	}

	return &resp.DealState, nil
}

// ProposeStorageDeal initiates the retrieval deal flow, which involves multiple requests and responses.
//
// This function is called after using ListProviders and QueryAs are used to identify an appropriate provider
// to store data. The parameters passed to ProposeStorageDeal should matched those returned by the miner from
// QueryAsk to ensure the greatest likelihood the provider will accept the deal.
//
// When called, the client takes the following actions:
//
// 1. Calculates the PieceCID for this deal from the given PayloadCID. (by writing the payload to a CAR file then calculating
// a merkle root for the resulting data)
//
// 2. Constructs a `DealProposal` (spec-actors type) with deal terms
//
// 3. Signs the `DealProposal` to make a ClientDealProposal
//
// 4. Gets the CID for the ClientDealProposal
//
// 5. Construct a ClientDeal to track the state of this deal.
//
// 6. Tells its statemachine to begin tracking the deal state by the CID of the ClientDealProposal
//
// 7. Triggers a `ClientEventOpen` event on its statemachine.
//
// 8. Records the Provider as a possible peer for retrieving this data in the future
//
// From then on, the statemachine controls the deal flow in the client. Other components may listen for events in this flow by calling
// `SubscribeToEvents` on the Client. The Client also provides access to the node and network and other functionality through
// its implementation of the Client FSM's ClientDealEnvironment.
//
// Documentation of the client state machine can be found at https://godoc.org/github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientstates
func (c *Client) ProposeStorageDeal(ctx context.Context, params storagemarket.ProposeStorageDealParams) (*storagemarket.ProposeStorageDealResult, error) {
	err := c.addMultiaddrs(ctx, params.Info.Address)
	if err != nil {
		return nil, xerrors.Errorf("looking up addresses: %w", err)
	}

	bs, err := c.bstores.Get(params.Data.Root)
	if err != nil {
		return nil, xerrors.Errorf("failed to get blockstore for imported root %s: %w", params.Data.Root, err)
	}

	commP, pieceSize, err := clientutils.CommP(ctx, bs, params.Data, c.maxTraversalLinks)
	if err != nil {
		return nil, xerrors.Errorf("computing commP failed: %w", err)
	}

	if uint64(pieceSize.Padded()) > params.Info.SectorSize {
		return nil, fmt.Errorf("cannot propose a deal whose piece size (%d) is greater than sector size (%d)", pieceSize.Padded(), params.Info.SectorSize)
	}

	pcMin := params.Collateral
	if pcMin.Int == nil || pcMin.IsZero() {
		pcMin, _, err = c.node.DealProviderCollateralBounds(ctx, pieceSize.Padded(), params.VerifiedDeal)
		if err != nil {
			return nil, xerrors.Errorf("computing deal provider collateral bound failed: %w", err)
		}
	}

	label, err := clientutils.LabelField(params.Data.Root)
	if err != nil {
		return nil, xerrors.Errorf("creating label field in proposal: %w", err)
	}

	dealProposal := market.DealProposal{
		PieceCID:             commP,
		PieceSize:            pieceSize.Padded(),
		Client:               params.Addr,
		Provider:             params.Info.Address,
		Label:                label,
		StartEpoch:           params.StartEpoch,
		EndEpoch:             params.EndEpoch,
		StoragePricePerEpoch: params.Price,
		ProviderCollateral:   pcMin,
		ClientCollateral:     big.Zero(),
		VerifiedDeal:         params.VerifiedDeal,
	}

	clientDealProposal, err := c.node.SignProposal(ctx, params.Addr, dealProposal)
	if err != nil {
		return nil, xerrors.Errorf("signing deal proposal failed: %w", err)
	}

	proposalNd, err := cborutil.AsIpld(clientDealProposal)
	if err != nil {
		return nil, xerrors.Errorf("getting proposal node failed: %w", err)
	}

	deal := &storagemarket.ClientDeal{
		ProposalCid:        proposalNd.Cid(),
		ClientDealProposal: *clientDealProposal,
		State:              storagemarket.StorageDealUnknown,
		Miner:              params.Info.PeerID,
		MinerWorker:        params.Info.Worker,
		DataRef:            params.Data,
		FastRetrieval:      params.FastRetrieval,
		DealStages:         storagemarket.NewDealStages(),
		CreationTime:       curTime(),
	}

	err = c.statemachines.Begin(proposalNd.Cid(), deal)
	if err != nil {
		return nil, xerrors.Errorf("setting up deal tracking: %w", err)
	}

	err = c.statemachines.Send(deal.ProposalCid, storagemarket.ClientEventOpen)
	if err != nil {
		return nil, xerrors.Errorf("initializing state machine: %w", err)
	}

	return &storagemarket.ProposeStorageDealResult{
			ProposalCid: deal.ProposalCid,
		}, c.discovery.AddPeer(ctx, params.Data.Root, retrievalmarket.RetrievalPeer{
			Address:  dealProposal.Provider,
			ID:       deal.Miner,
			PieceCID: &commP,
		})
}

func curTime() cbg.CborTime {
	now := time.Now()
	return cbg.CborTime(time.Unix(0, now.UnixNano()).UTC())
}

// GetPaymentEscrow returns the current funds available for deal payment
func (c *Client) GetPaymentEscrow(ctx context.Context, addr address.Address) (storagemarket.Balance, error) {
	tok, _, err := c.node.GetChainHead(ctx)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	return c.node.GetBalance(ctx, addr, tok)
}

// AddPaymentEscrow adds funds for storage deals
func (c *Client) AddPaymentEscrow(ctx context.Context, addr address.Address, amount abi.TokenAmount) error {
	done := make(chan error, 1)

	mcid, err := c.node.AddFunds(ctx, addr, amount)
	if err != nil {
		return err
	}

	err = c.node.WaitForMessage(ctx, mcid, func(code exitcode.ExitCode, bytes []byte, finalCid cid.Cid, err error) error {
		if err != nil {
			done <- xerrors.Errorf("AddFunds errored: %w", err)
		} else if code != exitcode.Ok {
			done <- xerrors.Errorf("AddFunds error, exit code: %s", code.String())
		} else {
			done <- nil
		}
		return nil
	})

	if err != nil {
		return err
	}

	return <-done
}

// SubscribeToEvents allows another component to listen for events on the StorageClient
// in order to track deals as they progress through the deal flow
func (c *Client) SubscribeToEvents(subscriber storagemarket.ClientSubscriber) shared.Unsubscribe {
	return shared.Unsubscribe(c.pubSub.Subscribe(subscriber))
}

// PollingInterval is a getter for the polling interval option
func (c *Client) PollingInterval() time.Duration {
	return c.pollingInterval
}

// Configure applies the given list of StorageClientOptions after a StorageClient
// is initialized
func (c *Client) Configure(options ...StorageClientOption) {
	for _, option := range options {
		option(c)
	}
}

func (c *Client) start(ctx context.Context) error {
	err := c.migrateStateMachines(ctx)
	publishErr := c.readySub.Publish(err)
	if publishErr != nil {
		log.Warnf("Publish storage client ready event: %s", err.Error())
	}
	if err != nil {
		return fmt.Errorf("Migrating storage client state machines: %w", err)
	}
	if err := c.restartDeals(ctx); err != nil {
		return fmt.Errorf("Failed to restart deals: %w", err)
	}
	return nil
}

func (c *Client) restartDeals(ctx context.Context) error {
	var deals []storagemarket.ClientDeal
	err := c.statemachines.List(&deals)
	if err != nil {
		return err
	}

	for _, deal := range deals {
		if c.statemachines.IsTerminated(deal) {
			continue
		}

		err = c.addMultiaddrs(ctx, deal.Proposal.Provider)
		if err != nil {
			return err
		}

		err = c.statemachines.Send(deal.ProposalCid, storagemarket.ClientEventRestart)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) dispatch(eventName fsm.EventName, deal fsm.StateType) {
	evt, ok := eventName.(storagemarket.ClientEvent)
	if !ok {
		log.Errorf("dropped bad event %s", eventName)
	}
	realDeal, ok := deal.(storagemarket.ClientDeal)
	if !ok {
		log.Errorf("not a ClientDeal %v", deal)
	}
	pubSubEvt := internalClientEvent{evt, realDeal}

	if err := c.pubSub.Publish(pubSubEvt); err != nil {
		log.Errorf("failed to publish event %d", evt)
	}
}

func (c *Client) verifyStatusResponseSignature(ctx context.Context, miner address.Address, response network.DealStatusResponse, origBytes []byte) (bool, error) {
	tok, _, err := c.node.GetChainHead(ctx)
	if err != nil {
		return false, xerrors.Errorf("getting chain head: %w", err)
	}

	valid, err := c.node.VerifySignature(ctx, response.Signature, miner, origBytes, tok)
	if err != nil {
		return false, xerrors.Errorf("validating signature: %w", err)
	}

	return valid, nil
}

func (c *Client) addMultiaddrs(ctx context.Context, providerAddr address.Address) error {
	tok, _, err := c.node.GetChainHead(ctx)
	if err != nil {
		return err
	}
	minfo, err := c.node.GetMinerInfo(ctx, providerAddr, tok)
	if err != nil {
		return err
	}

	if len(minfo.Addrs) > 0 {
		c.net.AddAddrs(minfo.PeerID, minfo.Addrs)
	}

	return nil
}

func newClientStateMachine(ds datastore.Batching, env fsm.Environment, notifier fsm.Notifier, storageMigrations versioning.VersionedMigrationList, target versioning.VersionKey) (fsm.Group, func(context.Context) error, error) {
	return versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
		Environment:     env,
		StateType:       storagemarket.ClientDeal{},
		StateKeyField:   "State",
		Events:          clientstates.ClientEvents,
		StateEntryFuncs: clientstates.ClientStateEntryFuncs,
		FinalityStates:  clientstates.ClientFinalityStates,
		Notifier:        notifier,
	}, storageMigrations, target)
}

type internalClientEvent struct {
	evt  storagemarket.ClientEvent
	deal storagemarket.ClientDeal
}

func clientDispatcher(evt pubsub.Event, fn pubsub.SubscriberFn) error {
	ie, ok := evt.(internalClientEvent)
	if !ok {
		return xerrors.New("wrong type of event")
	}
	cb, ok := fn.(storagemarket.ClientSubscriber)
	if !ok {
		return xerrors.New("wrong type of event")
	}
	log.Debugw("process storage client listeners", "name", storagemarket.ClientEvents[ie.evt], "proposal cid", ie.deal.ProposalCid)
	cb(ie.evt, ie.deal)
	return nil
}

// ClientFSMParameterSpec is a valid set of parameters for a client deal FSM - used in doc generation
var ClientFSMParameterSpec = fsm.Parameters{
	Environment:     &clientDealEnvironment{},
	StateType:       storagemarket.ClientDeal{},
	StateKeyField:   "State",
	Events:          clientstates.ClientEvents,
	StateEntryFuncs: clientstates.ClientStateEntryFuncs,
	FinalityStates:  clientstates.ClientFinalityStates,
}

var _ clientstates.ClientDealEnvironment = &clientDealEnvironment{}
