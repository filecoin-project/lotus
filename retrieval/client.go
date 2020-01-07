package retrieval

import (
	"context"
	"io"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	payapi "github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/paych"
	"github.com/filecoin-project/lotus/retrieval/discovery"
)

var log = logging.Logger("retrieval")

type Client struct {
	h host.Host

	pmgr   *paych.Manager
	payapi payapi.PaychAPI
}

func NewClient(h host.Host, pmgr *paych.Manager, payapi payapi.PaychAPI) *Client {
	return &Client{h: h, pmgr: pmgr, payapi: payapi}
}

func (c *Client) Query(ctx context.Context, p discovery.RetrievalPeer, data cid.Cid) api.QueryOffer {
	s, err := c.h.NewStream(ctx, p.ID, QueryProtocolID)
	if err != nil {
		log.Warn(err)
		return api.QueryOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
	}
	defer s.Close()

	err = cborutil.WriteCborRPC(s, &Query{
		Piece: data,
	})
	if err != nil {
		log.Warn(err)
		return api.QueryOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
	}

	var resp QueryResponse
	if err := resp.UnmarshalCBOR(s); err != nil {
		log.Warn(err)
		return api.QueryOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
	}

	return api.QueryOffer{
		Root:        data,
		Size:        resp.Size,
		MinPrice:    resp.MinPrice,
		Miner:       p.Address, // TODO: check
		MinerPeerID: p.ID,
	}
}

type clientStream struct {
	payapi payapi.PaychAPI
	stream network.Stream
	peeker cbg.BytePeeker

	root   cid.Cid
	size   types.BigInt
	offset uint64

	paych       address.Address
	lane        uint64
	total       types.BigInt
	transferred types.BigInt

	windowSize uint64 // how much we "trust" the peer
	verifier   BlockVerifier
}

// C > S
//
// Offset MUST be aligned on chunking boundaries, size is rounded up to leaf size
//
// > DealProposal{Mode: Unixfs0, RootCid, Offset, Size, Payment(nil if free)}
// < Resp{Accept}
// < ..(Intermediate Block)
// < ..Blocks
// < ..(Intermediate Block)
// < ..Blocks
// > DealProposal(...)
// < ...
func (c *Client) RetrieveUnixfs(ctx context.Context, root cid.Cid, size uint64, total types.BigInt, miner peer.ID, client, minerAddr address.Address, out io.Writer) error {
	s, err := c.h.NewStream(ctx, miner, ProtocolID)
	if err != nil {
		return xerrors.Errorf("failed to open stream to miner for retrieval query: %w", err)
	}
	defer s.Close()

	initialOffset := uint64(0) // TODO: Check how much data we have locally
	// TODO: Support in handler
	// TODO: Allow client to specify this

	paych, _, err := c.pmgr.GetPaych(ctx, client, minerAddr, total)
	if err != nil {
		return xerrors.Errorf("getting payment channel: %w", err)
	}
	lane, err := c.pmgr.AllocateLane(paych)
	if err != nil {
		return xerrors.Errorf("allocating payment lane: %w", err)
	}

	cst := clientStream{
		payapi: c.payapi,
		stream: s,
		peeker: cbg.GetPeeker(s),

		root:   root,
		size:   types.NewInt(size),
		offset: initialOffset,

		paych:       paych,
		lane:        lane,
		total:       total,
		transferred: types.NewInt(0),

		windowSize: build.UnixfsChunkSize,
		verifier:   &UnixFs0Verifier{Root: root},
	}

	for cst.offset != size+initialOffset {
		toFetch := cst.windowSize
		if toFetch+cst.offset > size {
			toFetch = size - cst.offset
		}
		log.Infof("Retrieve %dB @%d", toFetch, cst.offset)

		err := cst.doOneExchange(ctx, toFetch, out)
		if err != nil {
			return xerrors.Errorf("retrieval exchange: %w", err)
		}

		cst.offset += toFetch
	}
	return nil
}

func (cst *clientStream) doOneExchange(ctx context.Context, toFetch uint64, out io.Writer) error {
	payAmount := types.BigDiv(types.BigMul(cst.total, types.NewInt(toFetch)), cst.size)

	payment, err := cst.setupPayment(ctx, payAmount)
	if err != nil {
		return xerrors.Errorf("setting up retrieval payment: %w", err)
	}

	deal := &DealProposal{
		Payment: payment,
		Ref:     cst.root,
		Params: RetParams{
			Unixfs0: &Unixfs0Offer{
				Offset: cst.offset,
				Size:   toFetch,
			},
		},
	}

	if err := cborutil.WriteCborRPC(cst.stream, deal); err != nil {
		return xerrors.Errorf("sending incremental retrieval request: %w", err)
	}

	var resp DealResponse
	if err := cborutil.ReadCborRPC(cst.peeker, &resp); err != nil {
		return xerrors.Errorf("reading retrieval response: %w", err)
	}

	if resp.Status != Accepted {
		cst.windowSize = build.UnixfsChunkSize
		// TODO: apply some 'penalty' to miner 'reputation' (needs to be the same in both cases)

		if resp.Status == Error {
			return xerrors.Errorf("storage deal error: %s", resp.Message)
		}
		if resp.Status == Rejected {
			return xerrors.Errorf("storage deal rejected: %s", resp.Message)
		}
		return xerrors.New("storage deal response had no Accepted section")
	}

	log.Info("Retrieval accepted, fetching blocks")

	return cst.fetchBlocks(toFetch, out)

	// TODO: maybe increase miner window size after success
}

func (cst *clientStream) fetchBlocks(toFetch uint64, out io.Writer) error {
	blocksToFetch := (toFetch + build.UnixfsChunkSize - 1) / build.UnixfsChunkSize

	for i := uint64(0); i < blocksToFetch; {
		log.Infof("block %d of %d", i+1, blocksToFetch)

		var block Block
		if err := cborutil.ReadCborRPC(cst.peeker, &block); err != nil {
			return xerrors.Errorf("reading fetchBlock response: %w", err)
		}

		dataBlocks, err := cst.consumeBlockMessage(block, out)
		if err != nil {
			return xerrors.Errorf("consuming retrieved blocks: %w", err)
		}

		i += dataBlocks
	}

	return nil
}

func (cst *clientStream) consumeBlockMessage(block Block, out io.Writer) (uint64, error) {
	prefix, err := cid.PrefixFromBytes(block.Prefix)
	if err != nil {
		return 0, err
	}

	cid, err := prefix.Sum(block.Data)
	if err != nil {
		return 0, err
	}

	blk, err := blocks.NewBlockWithCid(block.Data, cid)
	if err != nil {
		return 0, err
	}

	internal, err := cst.verifier.Verify(context.TODO(), blk, out)
	if err != nil {
		log.Warnf("block verify failed: %s", err)
		return 0, err
	}

	// TODO: Smarter out, maybe add to filestore automagically
	//  (Also, persist intermediate nodes)

	if internal {
		return 0, nil
	}

	return 1, nil
}

func (cst *clientStream) setupPayment(ctx context.Context, toSend types.BigInt) (api.PaymentInfo, error) {
	amount := types.BigAdd(cst.transferred, toSend)

	sv, err := cst.payapi.PaychVoucherCreate(ctx, cst.paych, amount, cst.lane)
	if err != nil {
		return api.PaymentInfo{}, err
	}

	cst.transferred = amount

	return api.PaymentInfo{
		Channel:        cst.paych,
		ChannelMessage: nil,
		Vouchers:       []*types.SignedVoucher{sv},
	}, nil
}
