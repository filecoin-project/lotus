package node

import (
	"context"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
)

type API struct {
	Host   host.Host
	Chain  *chain.ChainStore
	PubSub *pubsub.PubSub
	Mpool  *chain.MessagePool
}

func (a *API) ChainSubmitBlock(ctx context.Context, blk *chain.BlockMsg) error {
	b, err := blk.Serialize()
	if err != nil {
		return err
	}

	// TODO: anything else to do here?
	return a.PubSub.Publish("/fil/blocks", b)
}

func (a *API) ChainHead(context.Context) ([]cid.Cid, error) {
	return a.Chain.GetHeaviestTipSet().Cids(), nil
}

func (a *API) ID(context.Context) (peer.ID, error) {
	return a.Host.ID(), nil
}

func (a *API) Version(context.Context) (api.Version, error) {
	return api.Version{
		Version: build.Version,
	}, nil
}

func (a *API) MpoolPending(context.Context) ([]*chain.SignedMessage, error) {
	return a.Mpool.Pending(), nil
}

func (a *API) NetPeers(context.Context) ([]peer.AddrInfo, error) {
	conns := a.Host.Network().Conns()
	out := make([]peer.AddrInfo, len(conns))

	for i, conn := range conns {
		out[i] = peer.AddrInfo{
			ID: conn.RemotePeer(),
			Addrs: []ma.Multiaddr{
				conn.RemoteMultiaddr(),
			},
		}
	}

	return out, nil
}

func (a *API) NetConnect(ctx context.Context, p peer.AddrInfo) error {
	return a.Host.Connect(ctx, p)
}

func (a *API) NetAddrsListen(context.Context) (peer.AddrInfo, error) {
	return peer.AddrInfo{
		ID:    a.Host.ID(),
		Addrs: a.Host.Addrs(),
	}, nil
}

var _ api.API = &API{}
