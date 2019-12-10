package deals

import (
	"context"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/cborutil"
	"github.com/filecoin-project/lotus/lib/statestore"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	retrievalmarket "github.com/filecoin-project/lotus/retrieval"
	"github.com/filecoin-project/lotus/retrieval/discovery"
)

var log = logging.Logger("deals")

type ClientDeal struct {
	ProposalCid cid.Cid
	Proposal    actors.StorageDealProposal
	State       api.DealState
	Miner       peer.ID
	MinerWorker address.Address
	DealID      uint64
	PayloadCid  cid.Cid

	PublishMessage *types.SignedMessage

	s inet.Stream
}

type Client struct {
	sm    *stmgr.StateManager
	chain *store.ChainStore
	h     host.Host
	w     *wallet.Wallet
	// dataTransfer
	// TODO: once the data transfer module is complete, the
	// client will listen to events on the data transfer module
	// Because we are using only a fake DAGService
	// implementation, there's no validation or events on the client side
	dataTransfer dtypes.ClientDataTransfer
	dag          dtypes.ClientDAG
	discovery    *discovery.Local
	events       *events.Events
	fm           *market.FundMgr

	deals *statestore.StateStore
	conns map[cid.Cid]inet.Stream

	incoming chan *ClientDeal
	updated  chan clientDealUpdate

	stop    chan struct{}
	stopped chan struct{}
}

type clientDealUpdate struct {
	newState api.DealState
	id       cid.Cid
	err      error
	mut      func(*ClientDeal)
}

type clientApi struct {
	full.ChainAPI
	full.StateAPI
}

func NewClient(sm *stmgr.StateManager, chain *store.ChainStore, h host.Host, w *wallet.Wallet, dag dtypes.ClientDAG, dataTransfer dtypes.ClientDataTransfer, discovery *discovery.Local, fm *market.FundMgr, deals dtypes.ClientDealStore, chainapi full.ChainAPI, stateapi full.StateAPI) *Client {
	c := &Client{
		sm:           sm,
		chain:        chain,
		h:            h,
		w:            w,
		dataTransfer: dataTransfer,
		dag:          dag,
		discovery:    discovery,
		fm:           fm,
		events:       events.NewEvents(context.TODO(), &clientApi{chainapi, stateapi}),

		deals: deals,
		conns: map[cid.Cid]inet.Stream{},

		incoming: make(chan *ClientDeal, 16),
		updated:  make(chan clientDealUpdate, 16),

		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	return c
}

func (c *Client) Run(ctx context.Context) {
	go func() {
		defer close(c.stopped)

		for {
			select {
			case deal := <-c.incoming:
				c.onIncoming(deal)
			case update := <-c.updated:
				c.onUpdated(ctx, update)
			case <-c.stop:
				return
			}
		}
	}()
}

func (c *Client) onIncoming(deal *ClientDeal) {
	log.Info("incoming deal")

	if _, ok := c.conns[deal.ProposalCid]; ok {
		log.Errorf("tracking deal connection: already tracking connection for deal %s", deal.ProposalCid)
		return
	}
	c.conns[deal.ProposalCid] = deal.s

	if err := c.deals.Begin(deal.ProposalCid, deal); err != nil {
		// We may have re-sent the proposal
		log.Errorf("deal tracking failed: %s", err)
		c.failDeal(deal.ProposalCid, err)
		return
	}

	go func() {
		c.updated <- clientDealUpdate{
			newState: api.DealUnknown,
			id:       deal.ProposalCid,
			err:      nil,
		}
	}()
}

func (c *Client) onUpdated(ctx context.Context, update clientDealUpdate) {
	log.Infof("Client deal %s updated state to %s", update.id, api.DealStates[update.newState])
	var deal ClientDeal
	err := c.deals.Mutate(update.id, func(d *ClientDeal) error {
		d.State = update.newState
		if update.mut != nil {
			update.mut(d)
		}
		deal = *d
		return nil
	})
	if update.err != nil {
		log.Errorf("deal %s failed: %s", update.id, update.err)
		c.failDeal(update.id, update.err)
		return
	}
	if err != nil {
		c.failDeal(update.id, err)
		return
	}

	switch update.newState {
	case api.DealUnknown: // new
		c.handle(ctx, deal, c.new, api.DealAccepted)
	case api.DealAccepted:
		c.handle(ctx, deal, c.accepted, api.DealStaged)
	case api.DealStaged:
		c.handle(ctx, deal, c.staged, api.DealSealing)
	case api.DealSealing:
		c.handle(ctx, deal, c.sealing, api.DealNoUpdate)
		// TODO: DealComplete -> watch for faults, expiration, etc.
	}
}

type ClientDealProposal struct {
	Data cid.Cid

	PricePerEpoch      types.BigInt
	ProposalExpiration uint64
	Duration           uint64

	ProviderAddress address.Address
	Client          address.Address
	MinerWorker     address.Address
	MinerID         peer.ID
}

func (c *Client) Start(ctx context.Context, p ClientDealProposal) (cid.Cid, error) {
	if err := c.fm.EnsureAvailable(ctx, p.Client, types.BigMul(p.PricePerEpoch, types.NewInt(p.Duration))); err != nil {
		return cid.Undef, xerrors.Errorf("adding market funds failed: %w", err)
	}

	commP, pieceSize, err := c.commP(ctx, p.Data)
	if err != nil {
		return cid.Undef, xerrors.Errorf("computing commP failed: %w", err)
	}

	dealProposal := &actors.StorageDealProposal{
		PieceRef:             commP,
		PieceSize:            uint64(pieceSize),
		Client:               p.Client,
		Provider:             p.ProviderAddress,
		ProposalExpiration:   p.ProposalExpiration,
		Duration:             p.Duration,
		StoragePricePerEpoch: p.PricePerEpoch,
		StorageCollateral:    types.NewInt(uint64(pieceSize)), // TODO: real calc
	}

	if err := api.SignWith(ctx, c.w.Sign, p.Client, dealProposal); err != nil {
		return cid.Undef, xerrors.Errorf("signing deal proposal failed: %w", err)
	}

	proposalNd, err := cborutil.AsIpld(dealProposal)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting proposal node failed: %w", err)
	}

	s, err := c.h.NewStream(ctx, p.MinerID, DealProtocolID)
	if err != nil {
		return cid.Undef, xerrors.Errorf("connecting to storage provider failed: %w", err)
	}

	proposal := &Proposal{
		DealProposal: dealProposal,
		Piece:        p.Data,
	}

	if err := cborutil.WriteCborRPC(s, proposal); err != nil {
		s.Reset()
		return cid.Undef, xerrors.Errorf("sending proposal to storage provider failed: %w", err)
	}

	deal := &ClientDeal{
		ProposalCid: proposalNd.Cid(),
		Proposal:    *dealProposal,
		State:       api.DealUnknown,
		Miner:       p.MinerID,
		MinerWorker: p.MinerWorker,
		PayloadCid:  p.Data,
		s:           s,
	}

	c.incoming <- deal

	return deal.ProposalCid, c.discovery.AddPeer(p.Data, retrievalmarket.RetrievalPeer{
		Address: dealProposal.Provider,
		ID:      deal.Miner,
	})
}

func (c *Client) QueryAsk(ctx context.Context, p peer.ID, a address.Address) (*types.SignedStorageAsk, error) {
	s, err := c.h.NewStream(ctx, p, AskProtocolID)
	if err != nil {
		return nil, xerrors.Errorf("failed to open stream to miner: %w", err)
	}

	req := &AskRequest{
		Miner: a,
	}
	if err := cborutil.WriteCborRPC(s, req); err != nil {
		return nil, xerrors.Errorf("failed to send ask request: %w", err)
	}

	var out AskResponse
	if err := cborutil.ReadCborRPC(s, &out); err != nil {
		return nil, xerrors.Errorf("failed to read ask response: %w", err)
	}

	if out.Ask == nil {
		return nil, xerrors.Errorf("got no ask back")
	}

	if out.Ask.Ask.Miner != a {
		return nil, xerrors.Errorf("got back ask for wrong miner")
	}

	if err := c.checkAskSignature(out.Ask); err != nil {
		return nil, xerrors.Errorf("ask was not properly signed")
	}

	return out.Ask, nil
}

func (c *Client) List() ([]ClientDeal, error) {
	var out []ClientDeal
	if err := c.deals.List(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetDeal(d cid.Cid) (*ClientDeal, error) {
	var out ClientDeal
	if err := c.deals.Get(d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Stop() {
	close(c.stop)
	<-c.stopped
}
