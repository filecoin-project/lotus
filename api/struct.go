package api

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// Struct implements API passing calls to user-provided function values.
type Struct struct {
	Internal struct {
		ID      func(context.Context) (peer.ID, error)
		Version func(context.Context) (Version, error)

		NetPeers       func(context.Context) ([]peer.AddrInfo, error)
		NetConnect     func(context.Context, peer.AddrInfo) error
		NetAddrsListen func(context.Context) ([]ma.Multiaddr, error)
	}
}

func (c *Struct) NetPeers(ctx context.Context) ([]peer.AddrInfo, error) {
	return c.Internal.NetPeers(ctx)
}

func (c *Struct) NetConnect(ctx context.Context, p peer.AddrInfo) error {
	return c.Internal.NetConnect(ctx, p)
}

func (c *Struct) NetAddrsListen(ctx context.Context) ([]ma.Multiaddr, error) {
	return c.Internal.NetAddrsListen(ctx)
}

// ID implements API.ID
func (c *Struct) ID(ctx context.Context) (peer.ID, error) {
	return c.Internal.ID(ctx)
}

// Version implements API.Version
func (c *Struct) Version(ctx context.Context) (Version, error) {
	return c.Internal.Version(ctx)
}

var _ API = &Struct{}
