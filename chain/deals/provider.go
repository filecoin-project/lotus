package deals

import (
	"context"
	"sync"

	cid "github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/filecoin-project/lotus/lib/statestore"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

type MinerDeal struct {
	Client      peer.ID
	Proposal    actors.StorageDealProposal
	ProposalCid cid.Cid
	State       api.DealState

	Ref cid.Cid

	DealID   uint64
	SectorID uint64 // Set when State >= DealStaged

	s inet.Stream
}

type Provider struct {
	pricePerByteBlock types.BigInt // how much we want for storing one byte for one block
	minPieceSize      uint64

	ask   *types.SignedStorageAsk
	askLk sync.Mutex

	secb   *sectorblocks.SectorBlocks
	sminer *storage.Miner
	full   api.FullNode

	// TODO: Use a custom protocol or graphsync in the future
	// TODO: GC
	dag dtypes.StagingDAG

	deals *statestore.StateStore
	ds    dtypes.MetadataDS

	conns map[cid.Cid]inet.Stream

	actor address.Address

	incoming chan MinerDeal
	updated  chan minerDealUpdate
	stop     chan struct{}
	stopped  chan struct{}
}

type minerDealUpdate struct {
	newState api.DealState
	id       cid.Cid
	err      error
	mut      func(*MinerDeal)
}

func NewProvider(ds dtypes.MetadataDS, sminer *storage.Miner, secb *sectorblocks.SectorBlocks, dag dtypes.StagingDAG, fullNode api.FullNode) (*Provider, error) {
	addr, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return nil, err
	}
	minerAddress, err := address.NewFromBytes(addr)
	if err != nil {
		return nil, err
	}

	h := &Provider{
		sminer: sminer,
		dag:    dag,
		full:   fullNode,
		secb:   secb,

		pricePerByteBlock: types.NewInt(3), // TODO: allow setting
		minPieceSize:      1,

		conns: map[cid.Cid]inet.Stream{},

		incoming: make(chan MinerDeal),
		updated:  make(chan minerDealUpdate),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),

		actor: minerAddress,

		deals: statestore.New(namespace.Wrap(ds, datastore.NewKey("/deals/client"))),
		ds:    ds,
	}

	if err := h.tryLoadAsk(); err != nil {
		return nil, err
	}

	if h.ask == nil {
		// TODO: we should be fine with this state, and just say it means 'not actively accepting deals'
		// for now... lets just set a price
		if err := h.SetPrice(types.NewInt(500_000_000), 1000000); err != nil {
			return nil, xerrors.Errorf("failed setting a default price: %w", err)
		}
	}

	return h, nil
}

func (p *Provider) Run(ctx context.Context) {
	// TODO: restore state

	go func() {
		defer log.Warn("quitting deal provider loop")
		defer close(p.stopped)

		for {
			select {
			case deal := <-p.incoming: // DealAccepted
				p.onIncoming(deal)
			case update := <-p.updated: // DealStaged
				p.onUpdated(ctx, update)
			case <-p.stop:
				return
			}
		}
	}()
}

func (p *Provider) onIncoming(deal MinerDeal) {
	log.Info("incoming deal")

	p.conns[deal.ProposalCid] = deal.s

	if err := p.deals.Begin(deal.ProposalCid, &deal); err != nil {
		// This can happen when client re-sends proposal
		p.failDeal(deal.ProposalCid, err)
		log.Errorf("deal tracking failed: %s", err)
		return
	}

	go func() {
		p.updated <- minerDealUpdate{
			newState: api.DealAccepted,
			id:       deal.ProposalCid,
			err:      nil,
		}
	}()
}

func (p *Provider) onUpdated(ctx context.Context, update minerDealUpdate) {
	log.Infof("Deal %s updated state to %d", update.id, update.newState)
	if update.err != nil {
		log.Errorf("deal %s (newSt: %d) failed: %+v", update.id, update.newState, update.err)
		p.failDeal(update.id, update.err)
		return
	}
	var deal MinerDeal
	err := p.deals.Mutate(update.id, func(d *MinerDeal) error {
		d.State = update.newState
		if update.mut != nil {
			update.mut(d)
		}
		deal = *d
		return nil
	})
	if err != nil {
		p.failDeal(update.id, err)
		return
	}

	switch update.newState {
	case api.DealAccepted:
		p.handle(ctx, deal, p.accept, api.DealStaged)
	case api.DealStaged:
		p.handle(ctx, deal, p.staged, api.DealSealing)
	case api.DealSealing:
		p.handle(ctx, deal, p.sealing, api.DealComplete)
	case api.DealComplete:
		p.handle(ctx, deal, p.complete, api.DealNoUpdate)
	}
}

func (p *Provider) newDeal(s inet.Stream, proposal actors.StorageDealProposal) (MinerDeal, error) {
	proposalNd, err := cborrpc.AsIpld(&proposal)
	if err != nil {
		return MinerDeal{}, err
	}

	ref, err := cid.Cast(proposal.PieceRef)
	if err != nil {
		return MinerDeal{}, err
	}

	return MinerDeal{
		Client:      s.Conn().RemotePeer(),
		Proposal:    proposal,
		ProposalCid: proposalNd.Cid(),
		State:       api.DealUnknown,

		Ref: ref,

		s: s,
	}, nil
}

func (p *Provider) HandleStream(s inet.Stream) {
	log.Info("Handling storage deal proposal!")

	proposal, err := p.readProposal(s)
	if err != nil {
		log.Error(err)
		s.Close()
		return
	}

	deal, err := p.newDeal(s, proposal)
	if err != nil {
		log.Error(err)
		s.Close()
		return
	}

	p.incoming <- deal
}

func (p *Provider) Stop() {
	close(p.stop)
	<-p.stopped
}
