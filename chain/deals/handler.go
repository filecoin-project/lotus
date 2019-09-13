package deals

import (
	"context"
	"math"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	cbor "github.com/ipfs/go-ipld-cbor"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/filecoin-project/go-lotus/storage/sectorblocks"
)

func init() {
	cbor.RegisterCborType(MinerDeal{})
}

type MinerDeal struct {
	Client      peer.ID
	Proposal    StorageDealProposal
	ProposalCid cid.Cid
	State       api.DealState

	Ref cid.Cid

	SectorID uint64 // Set when State >= DealStaged

	s inet.Stream
}

type Handler struct {
	pricePerByteBlock types.BigInt // how much we want for storing one byte for one block

	secst *sectorblocks.SectorBlocks
	full  api.FullNode

	// TODO: Use a custom protocol or graphsync in the future
	// TODO: GC
	dag dtypes.StagingDAG

	deals MinerStateStore
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

func NewHandler(ds dtypes.MetadataDS, secst *sectorblocks.SectorBlocks, dag dtypes.StagingDAG, fullNode api.FullNode) (*Handler, error) {
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

		pricePerByteBlock: types.NewInt(3), // TODO: allow setting

		conns: map[cid.Cid]inet.Stream{},

		incoming: make(chan MinerDeal),
		updated:  make(chan minerDealUpdate),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),

		actor: minerAddress,

		deals: MinerStateStore{StateStore{ds: namespace.Wrap(ds, datastore.NewKey("/deals/client"))}},
	}, nil
}

func (h *Handler) Run(ctx context.Context) {
	// TODO: restore state

	go func() {
		defer log.Error("quitting deal handler loop")
		defer close(h.stopped)

		for {
			select {
			case deal := <-h.incoming: // DealAccepted
				h.onIncoming(deal)
			case update := <-h.updated: // DealStaged
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
		h.updated <- minerDealUpdate{
			newState: api.DealAccepted,
			id:       deal.ProposalCid,
			err:      nil,
		}
	}()
}

func (h *Handler) onUpdated(ctx context.Context, update minerDealUpdate) {
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
	case api.DealAccepted:
		h.handle(ctx, deal, h.accept, api.DealStaged)
	case api.DealStaged:
		h.handle(ctx, deal, h.staged, api.DealSealing)
	case api.DealSealing:
		h.handle(ctx, deal, h.sealing, api.DealComplete)
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
		State:       api.DealUnknown,

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
