package deals

import (
	"context"
	"github.com/filecoin-project/lotus/node/impl/full"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/retrieval/discovery"
)

var log = logging.Logger("deals")

type ClientDeal struct {
	ProposalCid cid.Cid
	Proposal    actors.StorageDealProposal
	State       api.DealState
	Miner       peer.ID

	s inet.Stream
}

type Client struct {
	sm        *stmgr.StateManager
	chain     *store.ChainStore
	h         host.Host
	w         *wallet.Wallet
	dag       dtypes.ClientDAG
	discovery *discovery.Local
	mpool     full.MpoolAPI

	deals ClientStateStore
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
}

func NewClient(sm *stmgr.StateManager, chain *store.ChainStore, h host.Host, w *wallet.Wallet, ds dtypes.MetadataDS, dag dtypes.ClientDAG, discovery *discovery.Local, mpool full.MpoolAPI) *Client {
	c := &Client{
		sm:        sm,
		chain:     chain,
		h:         h,
		w:         w,
		dag:       dag,
		discovery: discovery,
		mpool:     mpool,

		deals: ClientStateStore{StateStore{ds: namespace.Wrap(ds, datastore.NewKey("/deals/client"))}},
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
	log.Infof("Deal %s updated state to %d", update.id, update.newState)
	var deal ClientDeal
	err := c.deals.MutateClient(update.id, func(d *ClientDeal) error {
		d.State = update.newState
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
		c.handle(ctx, deal, c.sealing, api.DealComplete)
	}
}

type ClientDealProposal struct {
	Data     cid.Cid
	DataSize uint64

	TotalPrice         types.BigInt
	ProposalExpiration uint64
	Duration           uint64

	ProviderAddress address.Address
	Client          address.Address
	MinerID         peer.ID
}

func (c *Client) Start(ctx context.Context, p ClientDealProposal) (cid.Cid, error) {
	// check market funds
	clientMarketBalance, err := c.sm.MarketBalance(ctx, p.Client, nil)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting client market balance failed: %w", err)
	}

	if clientMarketBalance.Available.LessThan(p.TotalPrice) {
		// TODO: move to a smarter market funds manager

		smsg, err := c.mpool.MpoolPushMessage(ctx, &types.Message{
			To:       actors.StorageMarketAddress,
			From:     p.Client,
			Value:    p.TotalPrice,
			GasPrice: types.NewInt(0),
			GasLimit: types.NewInt(1000000),
			Method:   actors.SMAMethods.AddBalance,
		})
		if err != nil {
			return cid.Undef, err
		}

		_, r, err := c.sm.WaitForMessage(ctx, smsg.Cid())
		if err != nil {
			return cid.Undef, err
		}

		if r.ExitCode != 0 {
			return cid.Undef, xerrors.Errorf("adding funds to storage miner market actor failed: exit %d", r.ExitCode)
		}
	}

	proposal := &actors.StorageDealProposal{
		PieceRef:           p.Data.Bytes(),
		PieceSize:          p.DataSize,
		PieceSerialization: actors.SerializationUnixFSv0,
		Client:             p.Client,
		Provider:           p.ProviderAddress,
		ProposalExpiration: p.ProposalExpiration,
		Duration:           p.Duration,
		StoragePrice:       p.TotalPrice,
		StorageCollateral:  types.NewInt(p.DataSize), // TODO: real calc
	}

	if err := api.SignWith(ctx, c.w.Sign, p.Client, proposal); err != nil {
		return cid.Undef, xerrors.Errorf("signing deal proposal failed: %w", err)
	}

	proposalNd, err := cborrpc.AsIpld(proposal)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting proposal node failed: %w", err)
	}

	s, err := c.h.NewStream(ctx, p.MinerID, DealProtocolID)
	if err != nil {
		s.Reset()
		return cid.Undef, xerrors.Errorf("connecting to storage provider failed: %w", err)
	}

	if err := cborrpc.WriteCborRPC(s, proposal); err != nil {
		s.Reset()
		return cid.Undef, xerrors.Errorf("sending proposal to storage provider failed: %w", err)
	}

	deal := &ClientDeal{
		ProposalCid: proposalNd.Cid(),
		Proposal:    *proposal,
		State:       api.DealUnknown,
		Miner:       p.MinerID,

		s: s,
	}

	c.incoming <- deal

	return deal.ProposalCid, c.discovery.AddPeer(p.Data, discovery.RetrievalPeer{
		Address: proposal.Provider,
		ID:      deal.Miner,
	})
}

func (c *Client) QueryAsk(ctx context.Context, p peer.ID, a address.Address) (*types.SignedStorageAsk, error) {
	s, err := c.h.NewStream(ctx, p, AskProtocolID)
	if err != nil {
		return nil, err
	}

	req := &AskRequest{
		Miner: a,
	}
	if err := cborrpc.WriteCborRPC(s, req); err != nil {
		return nil, xerrors.Errorf("failed to send ask request: %w", err)
	}

	var out AskResponse
	if err := cborrpc.ReadCborRPC(s, &out); err != nil {
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
	return c.deals.ListClient()
}

func (c *Client) Stop() {
	close(c.stop)
	<-c.stopped
}
