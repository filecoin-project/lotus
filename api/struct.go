package api

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
)

// Struct implements API passing calls to user-provided function values.
type Struct struct {
	Internal struct {
		ID      func(context.Context) (peer.ID, error)
		Version func(context.Context) (Version, error)

		ChainSubmitBlock func(ctx context.Context, blk *chain.BlockMsg) error
		ChainHead        func(context.Context) ([]cid.Cid, error)

		NetPeers       func(context.Context) ([]peer.AddrInfo, error)
		NetConnect     func(context.Context, peer.AddrInfo) error
		NetAddrsListen func(context.Context) (peer.AddrInfo, error)
	}
}

func (c *Struct) NetPeers(ctx context.Context) ([]peer.AddrInfo, error) {
	return c.Internal.NetPeers(ctx)
}

func (c *Struct) NetConnect(ctx context.Context, p peer.AddrInfo) error {
	return c.Internal.NetConnect(ctx, p)
}

func (c *Struct) NetAddrsListen(ctx context.Context) (peer.AddrInfo, error) {
	return c.Internal.NetAddrsListen(ctx)
}

func (c *Struct) ChainSubmitBlock(ctx context.Context, blk *chain.BlockMsg) error {
	return c.Internal.ChainSubmitBlock(ctx, blk)
}

func (c *Struct) ChainHead(ctx context.Context) ([]cid.Cid, error) {
	return c.Internal.ChainHead(ctx)
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
