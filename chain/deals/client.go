package deals

import (
	"context"
	"io"
	"io/ioutil"
	"math"
	"os"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	files "github.com/ipfs/go-ipfs-files"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/libp2p/go-libp2p-core/host"
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
	cs *store.ChainStore
	h  host.Host
	w  *wallet.Wallet
	dag dtypes.ClientDAG

	deals StateStore

	incoming chan ClientDeal

	stop    chan struct{}
	stopped chan struct{}
}

func NewClient(cs *store.ChainStore, h host.Host, w *wallet.Wallet, ds dtypes.MetadataDS, dag dtypes.ClientDAG) *Client {
	c := &Client{
		cs: cs,
		h:  h,
		w:  w,
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

func (c *Client) Start(ctx context.Context, data cid.Cid, totalPrice types.BigInt, from address.Address, miner address.Address, minerID peer.ID, blocksDuration uint64) (cid.Cid, error) {
	// TODO: Eww
	root, err := c.dag.Get(ctx, data)
	if err != nil {
		log.Errorf("failed to get file root for deal: %s", err)
		return cid.Undef, err
	}

	n, err := unixfile.NewUnixfsFile(ctx, c.dag, root)
	if err != nil {
		log.Errorf("cannot open unixfs file: %s", err)
		return cid.Undef, err
	}

	uf, ok := n.(files.File)
	if !ok {
		// TODO: we probably got directory, how should we handle this in unixfs mode?
		return cid.Undef, xerrors.New("unsupported unixfs type")
	}

	f, err := ioutil.TempFile(os.TempDir(), "commP-temp-")
	if err != nil {
		return cid.Undef, err
	}
	if _, err := io.Copy(f, uf); err != nil {
		return cid.Undef, err
	}
	if err := f.Close(); err != nil {
		return cid.Undef, err
	}
	commP, err := sectorbuilder.GeneratePieceCommitment(f.Name(), 6)
	if err != nil {
		return cid.Undef, err
	}
	if err := os.Remove(f.Name()); err != nil {
		return cid.Undef, err
	}

	dummyCid, _ := cid.Parse("bafkqaaa")

	// TODO: use data
	proposal := StorageDealProposal{
		PieceRef:          data.String(),
		SerializationMode: SerializationUnixFs,
		CommP:             commP[:],
		Size:              6,
		TotalPrice:        totalPrice,
		Duration:          blocksDuration,
		Payment: PaymentInfo{
			PayChActor:     address.Address{},
			Payer:          address.Address{},
			ChannelMessage: dummyCid,
			Vouchers:       nil,
		},
		MinerAddress:  miner,
		ClientAddress: from,
	}

	s, err := c.h.NewStream(ctx, minerID, ProtocolID)
	if err != nil {
		return cid.Undef, err
	}
	defer s.Close() // TODO: not too soon?

	log.Info("Sending deal proposal")

	msg, err := cbor.DumpObject(proposal)
	if err != nil {
		return cid.Undef, err
	}
	sig, err := c.w.Sign(from, msg)
	if err != nil {
		return cid.Undef, err
	}

	signedProposal := &SignedStorageDealProposal{
		Proposal:  proposal,
		Signature: sig,
	}

	if err := cborrpc.WriteCborRPC(s, signedProposal); err != nil {
		return cid.Undef, err
	}

	log.Info("Reading response")

	var resp SignedStorageDealResponse
	if err := cborrpc.ReadCborRPC(s, &resp); err != nil {
		log.Errorw("failed to read StorageDealResponse message", "error", err)
		return cid.Undef, err
	}

	// TODO: verify signature

	if resp.Response.State != Accepted {
		return cid.Undef, xerrors.Errorf("Deal wasn't accepted (State=%d)", resp.Response.State)
	}

	log.Info("Registering deal")

	proposalNd, err := cbor.WrapObject(proposal, math.MaxUint64, -1)
	if err != nil {
		return cid.Undef, err
	}

	deal := ClientDeal{
		ProposalCid: proposalNd.Cid(),
		Status:      DealResolvingMiner,
		Miner:       minerID,
	}

	c.incoming <- deal
	return proposalNd.Cid(), nil
}

func (c *Client) Stop() {
	close(c.stop)
	<-c.stopped
}
