package deals

import (
	"context"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"math"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	files "github.com/ipfs/go-ipfs-files"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
)

func init() {
	cbor.RegisterCborType(ClientDeal{})
}

var log = logging.Logger("deals")

const ProtocolID = "/fil/storage/mk/1.0.0"

type DealStatus int

const (
	DealResolvingMiner = DealStatus(iota)
)

type ClientDeal struct {
	ProposalCid cid.Cid
	Status      DealStatus
	Miner       peer.ID
}

type Client struct {
	cs  *store.ChainStore
	h   host.Host
	w   *wallet.Wallet
	dag dtypes.ClientDAG

	deals StateStore

	incoming chan ClientDeal

	stop    chan struct{}
	stopped chan struct{}
}

func NewClient(cs *store.ChainStore, h host.Host, w *wallet.Wallet, ds dtypes.MetadataDS, dag dtypes.ClientDAG) *Client {
	c := &Client{
		cs:  cs,
		h:   h,
		w:   w,
		dag: dag,

		deals: StateStore{ds: namespace.Wrap(ds, datastore.NewKey("/deals/client"))},

		incoming: make(chan ClientDeal, 16),

		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	return c
}

func (c *Client) Run() {
	go func() {
		defer close(c.stopped)

		for {
			select {
			case deal := <-c.incoming:
				log.Info("incoming deal")

				// TODO: track in datastore
				if err := c.deals.Begin(deal.ProposalCid, deal); err != nil {
					log.Errorf("deal state begin failed: %s", err)
					continue
				}

			case <-c.stop:
				return
			}
		}
	}()
}

func (c *Client) commP(ctx context.Context, data cid.Cid) ([]byte, int64, error) {
	root, err := c.dag.Get(ctx, data)
	if err != nil {
		log.Errorf("failed to get file root for deal: %s", err)
		return nil, 0, err
	}

	n, err := unixfile.NewUnixfsFile(ctx, c.dag, root)
	if err != nil {
		log.Errorf("cannot open unixfs file: %s", err)
		return nil, 0, err
	}

	uf, ok := n.(files.File)
	if !ok {
		// TODO: we probably got directory, how should we handle this in unixfs mode?
		return nil, 0, xerrors.New("unsupported unixfs type")
	}

	size, err := uf.Size()
	if err != nil {
		return nil, 0, err
	}

	var commP [sectorbuilder.CommitmentBytesLen]byte
	err = withTemp(uf, func(f string) error {
		commP, err = sectorbuilder.GeneratePieceCommitment(f, uint64(size))
		return err
	})
	return commP[:], size, err
}

func (c *Client) sendProposal(s inet.Stream, proposal StorageDealProposal, from address.Address) error {
	log.Info("Sending deal proposal")

	msg, err := cbor.DumpObject(proposal)
	if err != nil {
		return err
	}
	sig, err := c.w.Sign(from, msg)
	if err != nil {
		return err
	}

	signedProposal := &SignedStorageDealProposal{
		Proposal:  proposal,
		Signature: sig,
	}

	return cborrpc.WriteCborRPC(s, signedProposal)
}

func (c *Client) waitAccept(s inet.Stream, proposal StorageDealProposal, minerID peer.ID) (ClientDeal, error) {
	log.Info("Waiting for response")

	var resp SignedStorageDealResponse
	if err := cborrpc.ReadCborRPC(s, &resp); err != nil {
		log.Errorw("failed to read StorageDealResponse message", "error", err)
		return ClientDeal{}, err
	}

	// TODO: verify signature

	if resp.Response.State != Accepted {
		return ClientDeal{}, xerrors.Errorf("Deal wasn't accepted (State=%d)", resp.Response.State)
	}

	proposalNd, err := cbor.WrapObject(proposal, math.MaxUint64, -1)
	if err != nil {
		return ClientDeal{}, err
	}

	if resp.Response.Proposal != proposalNd.Cid() {
		return ClientDeal{}, xerrors.New("miner responded to a wrong proposal")
	}

	return ClientDeal{
		ProposalCid: proposalNd.Cid(),
		Status:      DealResolvingMiner,
		Miner:       minerID,
	}, nil
}

type ClientDealProposal struct {
	Data cid.Cid

	TotalPrice types.BigInt
	Duration   uint64

	Payment actors.PaymentInfo

	MinerAddress  address.Address
	ClientAddress address.Address
	MinerID       peer.ID
}

func (c *Client) VerifyParams(ctx context.Context, data cid.Cid) (*actors.StorageVoucherData, error) {
	commP, size, err := c.commP(ctx, data)
	if err != nil {
		return nil, err
	}

	return &actors.StorageVoucherData{
		CommP:     commP,
		PieceSize: types.NewInt(uint64(size)),
	}, nil
}

func (c *Client) Start(ctx context.Context, p ClientDealProposal, vd *actors.StorageVoucherData) (cid.Cid, error) {
	// TODO: use data
	proposal := StorageDealProposal{
		PieceRef:          p.Data.String(),
		SerializationMode: SerializationUnixFs,
		CommP:             vd.CommP[:],
		Size:              vd.PieceSize.Uint64(),
		TotalPrice:        p.TotalPrice,
		Duration:          p.Duration,
		Payment:           p.Payment,
		MinerAddress:      p.MinerAddress,
		ClientAddress:     p.ClientAddress,
	}

	s, err := c.h.NewStream(ctx, p.MinerID, ProtocolID)
	if err != nil {
		return cid.Undef, err
	}
	defer s.Reset() // TODO: handle other updates

	if err := c.sendProposal(s, proposal, p.ClientAddress); err != nil {
		return cid.Undef, err
	}

	deal, err := c.waitAccept(s, proposal, p.MinerID)
	if err != nil {
		return cid.Undef, err
	}

	log.Info("DEAL ACCEPTED!")

	// TODO: actually care about what happens with the deal after it was accepted
	//c.incoming <- deal
	return deal.ProposalCid, nil
}

func (c *Client) Stop() {
	close(c.stop)
	<-c.stopped
}
