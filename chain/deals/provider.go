package deals

import (
	"context"
	"errors"
	"sync"

	cid "github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-components/datatransfer"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborutil"
	"github.com/filecoin-project/lotus/lib/statestore"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

var ProviderDsPrefix = "/deals/provider"

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

	// TODO: This will go away once storage market module + CAR
	// is implemented
	dag dtypes.StagingDAG

	// dataTransfer is the manager of data transfers used by this storage provider
	dataTransfer dtypes.ProviderDataTransfer

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

var (
	// ErrDataTransferFailed means a data transfer for a deal failed
	ErrDataTransferFailed = errors.New("deal data transfer failed")
)

func NewProvider(ds dtypes.MetadataDS, sminer *storage.Miner, secb *sectorblocks.SectorBlocks, dag dtypes.StagingDAG, dataTransfer dtypes.ProviderDataTransfer, fullNode api.FullNode) (*Provider, error) {
	addr, err := ds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return nil, err
	}
	minerAddress, err := address.NewFromBytes(addr)
	if err != nil {
		return nil, err
	}

	h := &Provider{
		sminer:       sminer,
		dag:          dag,
		dataTransfer: dataTransfer,
		full:         fullNode,
		secb:         secb,

		pricePerByteBlock: types.NewInt(3), // TODO: allow setting
		minPieceSize:      256,             // TODO: allow setting (BUT KEEP MIN 256! (because of how we fill sectors up))

		conns: map[cid.Cid]inet.Stream{},

		incoming: make(chan MinerDeal),
		updated:  make(chan minerDealUpdate),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),

		actor: minerAddress,

		deals: statestore.New(namespace.Wrap(ds, datastore.NewKey(ProviderDsPrefix))),
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

	// register a data transfer event handler -- this will move deals from
	// accepted to staged
	h.dataTransfer.SubscribeToEvents(h.onDataTransferEvent)

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
	log.Infof("Deal %s updated state to %s", update.id, api.DealStates[update.newState])
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
		p.handle(ctx, deal, p.accept, api.DealNoUpdate)
	case api.DealStaged:
		p.handle(ctx, deal, p.staged, api.DealSealing)
	case api.DealSealing:
		p.handle(ctx, deal, p.sealing, api.DealComplete)
	case api.DealComplete:
		p.handle(ctx, deal, p.complete, api.DealNoUpdate)
	}
}

// onDataTransferEvent is the function called when an event occurs in a data
// transfer -- it reads the voucher to verify this even occurred in a storage
// market deal, then, based on the data transfer event that occurred, it generates
// and update message for the deal -- either moving to staged for a completion
// event or moving to error if a data transfer error occurs
func (p *Provider) onDataTransferEvent(event datatransfer.Event, channelState datatransfer.ChannelState) {
	voucher, ok := channelState.Voucher().(*StorageDataTransferVoucher)
	// if this event is for a transfer not related to storage, ignore
	if !ok {
		return
	}

	// data transfer events for opening and progress do not affect deal state
	var next api.DealState
	var err error
	var mut func(*MinerDeal)
	switch event.Code {
	case datatransfer.Complete:
		next = api.DealStaged
		mut = func(deal *MinerDeal) {
			deal.DealID = voucher.DealID
		}
	case datatransfer.Error:
		next = api.DealFailed
		err = ErrDataTransferFailed
	default:
		// the only events we care about are complete and error
		return
	}

	select {
	case p.updated <- minerDealUpdate{
		newState: next,
		id:       voucher.Proposal,
		err:      err,
		mut:      mut,
	}:
	case <-p.stop:
	}
}

func (p *Provider) newDeal(s inet.Stream, proposal Proposal) (MinerDeal, error) {
	proposalNd, err := cborutil.AsIpld(proposal.DealProposal)
	if err != nil {
		return MinerDeal{}, err
	}

	return MinerDeal{
		Client:      s.Conn().RemotePeer(),
		Proposal:    *proposal.DealProposal,
		ProposalCid: proposalNd.Cid(),
		State:       api.DealUnknown,

		Ref: proposal.Piece,

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
		log.Errorf("%+v", err)
		s.Close()
		return
	}

	p.incoming <- deal
}

func (p *Provider) Stop() {
	close(p.stop)
	<-p.stopped
}
