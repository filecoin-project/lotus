package retrieval

import (
	"context"
	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	"io/ioutil"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/retrieval/discovery"
)

var log = logging.Logger("retrieval")

type Client struct {
	h host.Host
}

func NewClient(h host.Host) *Client {
	return &Client{h: h}
}

func (c *Client) Query(ctx context.Context, p discovery.RetrievalPeer, data cid.Cid) api.RetrievalOffer {
	s, err := c.h.NewStream(ctx, p.ID, QueryProtocolID)
	if err != nil {
		log.Warn(err)
		return api.RetrievalOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
	}
	defer s.Close()

	err = cborrpc.WriteCborRPC(s, RetQuery{
		Piece: data,
	})
	if err != nil {
		log.Warn(err)
		return api.RetrievalOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
	}

	// TODO: read deadline
	rawResp, err := ioutil.ReadAll(s)
	if err != nil {
		log.Warn(err)
		return api.RetrievalOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
	}

	var resp RetQueryResponse
	if err := cbor.DecodeInto(rawResp, &resp); err != nil {
		log.Warn(err)
		return api.RetrievalOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
	}

	return api.RetrievalOffer{
		Size:        resp.Size,
		MinPrice:    resp.MinPrice,
		Miner:       p.Address, // TODO: check
		MinerPeerID: p.ID,
	}
}
