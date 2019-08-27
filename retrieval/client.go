package retrieval

import (
	"context"
	"io/ioutil"

	pb "github.com/ipfs/go-bitswap/message/pb"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-msgio"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	"github.com/filecoin-project/go-lotus/retrieval/discovery"
)

var log = logging.Logger("retrieval")

type Client struct {
	h host.Host
}

func NewClient(h host.Host) *Client {
	return &Client{h: h}
}

func (c *Client) Query(ctx context.Context, p discovery.RetrievalPeer, data cid.Cid) api.QueryOffer {
	s, err := c.h.NewStream(ctx, p.ID, QueryProtocolID)
	if err != nil {
		log.Warn(err)
		return api.QueryOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
	}
	defer s.Close()

	err = cborrpc.WriteCborRPC(s, Query{
		Piece: data,
	})
	if err != nil {
		log.Warn(err)
		return api.QueryOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
	}

	// TODO: read deadline
	rawResp, err := ioutil.ReadAll(s)
	if err != nil {
		log.Warn(err)
		return api.QueryOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
	}

	var resp QueryResponse
	if err := cbor.DecodeInto(rawResp, &resp); err != nil {
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
	stream network.Stream

	root   cid.Cid
	offset uint64

	windowSize uint64 // how much we "trust" the peer
	verifier   BlockVerifier
}

// C > S
//
// Offset MUST be aligned on chunking boundaries, size is rounded up to leaf size
//
// > Deal{Mode: Unixfs0, RootCid, Offset, Size, Payment(nil if free)}
// < Resp{Accept}
// < ..(Intermediate Block)
// < ..Blocks
// < ..(Intermediate Block)
// < ..Blocks
// > Deal(...)
// < ...
func (c *Client) RetrieveUnixfs(ctx context.Context, root cid.Cid, size uint64, miner peer.ID, minerAddr address.Address) error {
	s, err := c.h.NewStream(ctx, miner, ProtocolID)
	if err != nil {
		return err
	}
	defer s.Close()

	cst := clientStream{
		stream: s,

		root:   root,
		offset: 0, // TODO: Check how much data we have locally
		// TODO: Support in handler
		// TODO: Allow client to specify this

		windowSize: build.UnixfsChunkSize,
		verifier:   &OptimisticVerifier{}, // TODO: Use a real verifier
	}

	for cst.offset != size {
		toFetch := cst.windowSize
		if toFetch+cst.offset > size {
			toFetch = size - cst.offset
		}

		err := cst.doOneExchange(toFetch)
		if err != nil {
			return err
		}

		cst.offset += toFetch
	}
	log.Info("RETRIEVE SUCCESSFUL")
	return nil
}

func (cst *clientStream) doOneExchange(toFetch uint64) error {
	deal := Deal{Unixfs0: &Unixfs0Offer{
		Root:   cst.root,
		Offset: cst.offset,
		Size:   toFetch,
	}}

	if err := cborrpc.WriteCborRPC(cst.stream, deal); err != nil {
		return err
	}

	var resp DealResponse
	if err := cborrpc.ReadCborRPC(cst.stream, &resp); err != nil {
		log.Error(err)
		return err
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

	return cst.fetchBlocks(toFetch)

	// TODO: maybe increase miner window size after success
}

func (cst *clientStream) fetchBlocks(toFetch uint64) error {
	blocksToFetch := (toFetch + build.UnixfsChunkSize - 1) / build.UnixfsChunkSize

	// TODO: put msgio into spec
	reader := msgio.NewVarintReaderSize(cst.stream, network.MessageSizeMax)

	for i := uint64(0); i < blocksToFetch; {
		msg, err := reader.ReadMsg()
		if err != nil {
			return err
		}

		var pb pb.Message_Block
		if err := pb.Unmarshal(msg); err != nil {
			return err
		}

		dataBlocks, err := cst.consumeBlockMessage(pb)
		if err != nil {
			return err
		}

		i += dataBlocks

		reader.ReleaseMsg(msg)
	}

	return nil
}

func (cst *clientStream) consumeBlockMessage(pb pb.Message_Block) (uint64, error) {
	prefix, err := cid.PrefixFromBytes(pb.GetPrefix())
	if err != nil {
		return 0, err
	}
	cid, err := prefix.Sum(pb.GetData())

	blk, err := blocks.NewBlockWithCid(pb.GetData(), cid)
	if err != nil {
		return 0, err
	}

	internal, err := cst.verifier.Verify(blk)
	if err != nil {
		return 0, err
	}

	// TODO: Persist block

	if internal {
		return 0, nil
	}
	return 1, nil
}
