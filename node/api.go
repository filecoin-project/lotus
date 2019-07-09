package node

import (
	"context"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type API struct {
	Host host.Host
}

func (a *API) ID(context.Context) (peer.ID, error) {
	return a.Host.ID(), nil
}

func (a *API) Version(context.Context) (api.Version, error) {
	return api.Version{
		Version: build.Version,
	}, nil
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

func (a *API) NetAddrsListen(context.Context) (api.MultiaddrSlice, error) {
	return a.Host.Addrs(), nil
}

var _ api.API = &API{}
