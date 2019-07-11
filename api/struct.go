package api

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/address"

	"github.com/libp2p/go-libp2p-core/peer"
)

// Struct implements API passing calls to user-provided function values.
type Struct struct {
	Internal struct {
		ID      func(context.Context) (peer.ID, error)
		Version func(context.Context) (Version, error)

		ChainSubmitBlock   func(ctx context.Context, blk *chain.BlockMsg) error
		ChainHead          func(context.Context) (*chain.TipSet, error)
		ChainGetRandomness func(context.Context, *chain.TipSet) ([]byte, error)

		MpoolPending func(context.Context, *chain.TipSet) ([]*chain.SignedMessage, error)

		MinerStart       func(context.Context, address.Address) error
		MinerCreateBlock func(context.Context, address.Address, *chain.TipSet, []chain.Ticket, chain.ElectionProof, []*chain.SignedMessage) (*chain.BlockMsg, error)

		NetPeers       func(context.Context) ([]peer.AddrInfo, error)
		NetConnect     func(context.Context, peer.AddrInfo) error
		NetAddrsListen func(context.Context) (peer.AddrInfo, error)
	}
}

func (c *Struct) MpoolPending(ctx context.Context, ts *chain.TipSet) ([]*chain.SignedMessage, error) {
	return c.Internal.MpoolPending(ctx, ts)
}

func (c *Struct) MinerStart(ctx context.Context, addr address.Address) error {
	return c.Internal.MinerStart(ctx, addr)
}

func (c *Struct) MinerCreateBlock(ctx context.Context, addr address.Address, base *chain.TipSet, tickets []chain.Ticket, eproof chain.ElectionProof, msgs []*chain.SignedMessage) (*chain.BlockMsg, error) {
	return c.Internal.MinerCreateBlock(ctx, addr, base, tickets, eproof, msgs)
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

func (c *Struct) ChainHead(ctx context.Context) (*chain.TipSet, error) {
	return c.Internal.ChainHead(ctx)
}

func (c *Struct) ChainGetRandomness(ctx context.Context, pts *chain.TipSet) ([]byte, error) {
	return c.Internal.ChainGetRandomness(ctx, pts)
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
