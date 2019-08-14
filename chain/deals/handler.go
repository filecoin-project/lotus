package deals

import (
	"context"
	"math"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/filecoin-project/go-lotus/storage/sector"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	cbor "github.com/ipfs/go-ipld-cbor"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

func init() {
	cbor.RegisterCborType(MinerDeal{})
}

type MinerDeal struct {
	Client      peer.ID
	Proposal    StorageDealProposal
	ProposalCid cid.Cid
	State       DealState

	Ref cid.Cid

	SectorID uint64 // Set when State >= Staged

	s inet.Stream
}

type Handler struct {
	secst *sector.Store
	full  api.FullNode

	// TODO: Use a custom protocol or graphsync in the future
	// TODO: GC
	dag dtypes.StagingDAG

	deals StateStore
	conns map[cid.Cid]inet.Stream

	actor address.Address

	incoming chan MinerDeal
	updated  chan dealUpdate
	stop     chan struct{}
	stopped  chan struct{}
}

type dealUpdate struct {
	newState DealState
	id       cid.Cid
	err      error
	mut      func(*MinerDeal)
}

func NewHandler(ds dtypes.MetadataDS, secst *sector.Store, dag dtypes.StagingDAG, fullNode api.FullNode) (*Handler, error) {
	addr, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return nil, err
	}
	minerAddress, err := address.NewFromBytes(addr)
	if err != nil {
		return nil, err
	}

	return &Handler{
		secst: secst,
		dag:   dag,
		full:  fullNode,

		conns: map[cid.Cid]inet.Stream{},

		incoming: make(chan MinerDeal),
		updated:  make(chan dealUpdate),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),

		actor: minerAddress,

		deals: StateStore{ds: namespace.Wrap(ds, datastore.NewKey("/deals/client"))},
	}, nil
}

func (h *Handler) Run(ctx context.Context) {
	// TODO: restore state

	go func() {
		defer log.Error("quitting deal handler loop")
		defer close(h.stopped)

		for {
			select {
			case deal := <-h.incoming: // Accepted
				h.onIncoming(deal)
			case update := <-h.updated: // Staged
				h.onUpdated(ctx, update)
			case <-h.stop:
				return
			}
		}
	}()
}

func (h *Handler) onIncoming(deal MinerDeal) {
	log.Info("incoming deal")

	h.conns[deal.ProposalCid] = deal.s

	if err := h.deals.Begin(deal.ProposalCid, deal); err != nil {
		// This can happen when client re-sends proposal
		h.failDeal(deal.ProposalCid, err)
		log.Errorf("deal tracking failed: %s", err)
		return
	}

	go func() {
		h.updated <- dealUpdate{
			newState: Accepted,
			id:       deal.ProposalCid,
			err:      nil,
		}
	}()
}

func (h *Handler) onUpdated(ctx context.Context, update dealUpdate) {
	log.Infof("Deal %s updated state to %d", update.id, update.newState)
	if update.err != nil {
		log.Errorf("deal %s failed: %s", update.id, update.err)
		h.failDeal(update.id, update.err)
		return
	}
	var deal MinerDeal
	err := h.deals.MutateMiner(update.id, func(d *MinerDeal) error {
		d.State = update.newState
		if update.mut != nil {
			update.mut(d)
		}
		deal = *d
		return nil
	})
	if err != nil {
		h.failDeal(update.id, err)
		return
	}

	switch update.newState {
	case Accepted:
		h.handle(ctx, deal, h.accept, Staged)
	case Staged:
		h.handle(ctx, deal, h.staged, Sealing)
	case Sealing:
		h.handle(ctx, deal, h.sealing, Complete)
	}
}

func (h *Handler) newDeal(s inet.Stream, proposal StorageDealProposal) (MinerDeal, error) {
	// TODO: Review: Not signed?
	proposalNd, err := cbor.WrapObject(proposal, math.MaxUint64, -1)
	if err != nil {
		return MinerDeal{}, err
	}

	ref, err := cid.Parse(proposal.PieceRef)
	if err != nil {
		return MinerDeal{}, err
	}

	return MinerDeal{
		Client:      s.Conn().RemotePeer(),
		Proposal:    proposal,
		ProposalCid: proposalNd.Cid(),
		State:       Unknown,

		Ref: ref,

		s: s,
	}, nil
}

func (h *Handler) HandleStream(s inet.Stream) {
	log.Info("Handling storage deal proposal!")

	proposal, err := h.readProposal(s)
	if err != nil {
		log.Error(err)
		s.Close()
		return
	}

	deal, err := h.newDeal(s, proposal.Proposal)
	if err != nil {
		log.Error(err)
		s.Close()
		return
	}

	h.incoming <- deal
}

func (h *Handler) Stop() {
	close(h.stop)
	<-h.stopped
}
