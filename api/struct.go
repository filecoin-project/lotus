package api

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
)

// Struct implements API passing calls to user-provided function values.
type Struct struct {
	Internal struct {
		ID      func(context.Context) (peer.ID, error)
		Version func(context.Context) (Version, error)

		ChainSubmitBlock      func(ctx context.Context, blk *chain.BlockMsg) error
		ChainHead             func(context.Context) (*chain.TipSet, error)
		ChainGetRandomness    func(context.Context, *chain.TipSet) ([]byte, error)
		ChainWaitMsg          func(context.Context, cid.Cid) (*MsgWait, error)
		ChainGetBlock         func(context.Context, cid.Cid) (*chain.BlockHeader, error)
		ChainGetBlockMessages func(context.Context, cid.Cid) ([]*chain.SignedMessage, error)

		MpoolPending func(context.Context, *chain.TipSet) ([]*chain.SignedMessage, error)
		MpoolPush    func(context.Context, *chain.SignedMessage) error

		MinerStart       func(context.Context, address.Address) error
		MinerCreateBlock func(context.Context, address.Address, *chain.TipSet, []chain.Ticket, chain.ElectionProof, []*chain.SignedMessage) (*chain.BlockMsg, error)

		WalletNew            func(context.Context, string) (address.Address, error)
		WalletList           func(context.Context) ([]address.Address, error)
		WalletBalance        func(context.Context, address.Address) (types.BigInt, error)
		WalletSign           func(context.Context, address.Address, []byte) (*chain.Signature, error)
		WalletDefaultAddress func(context.Context) (address.Address, error)
		MpoolGetNonce        func(context.Context, address.Address) (uint64, error)

		ClientImport      func(ctx context.Context, path string) (cid.Cid, error)
		ClientListImports func(ctx context.Context) ([]Import, error)

		NetPeers       func(context.Context) ([]peer.AddrInfo, error)
		NetConnect     func(context.Context, peer.AddrInfo) error
		NetAddrsListen func(context.Context) (peer.AddrInfo, error)
	}
}

func (c *Struct) ClientListImports(ctx context.Context) ([]Import, error) {
	return c.Internal.ClientListImports(ctx)
}

func (c *Struct) ClientImport(ctx context.Context, path string) (cid.Cid, error) {
	return c.Internal.ClientImport(ctx, path)
}

func (c *Struct) MpoolPending(ctx context.Context, ts *chain.TipSet) ([]*chain.SignedMessage, error) {
	return c.Internal.MpoolPending(ctx, ts)
}

func (c *Struct) MpoolPush(ctx context.Context, smsg *chain.SignedMessage) error {
	return c.Internal.MpoolPush(ctx, smsg)
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

func (c *Struct) ChainWaitMsg(ctx context.Context, msgc cid.Cid) (*MsgWait, error) {
	return c.Internal.ChainWaitMsg(ctx, msgc)
}

// ID implements API.ID
func (c *Struct) ID(ctx context.Context) (peer.ID, error) {
	return c.Internal.ID(ctx)
}

// Version implements API.Version
func (c *Struct) Version(ctx context.Context) (Version, error) {
	return c.Internal.Version(ctx)
}

func (c *Struct) WalletNew(ctx context.Context, typ string) (address.Address, error) {
	return c.Internal.WalletNew(ctx, typ)
}

func (c *Struct) WalletList(ctx context.Context) ([]address.Address, error) {
	return c.Internal.WalletList(ctx)
}

func (c *Struct) WalletBalance(ctx context.Context, a address.Address) (types.BigInt, error) {
	return c.Internal.WalletBalance(ctx, a)
}

func (c *Struct) WalletSign(ctx context.Context, k address.Address, msg []byte) (*chain.Signature, error) {
	return c.Internal.WalletSign(ctx, k, msg)
}

func (c *Struct) WalletDefaultAddress(ctx context.Context) (address.Address, error) {
	return c.Internal.WalletDefaultAddress(ctx)
}

func (c *Struct) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return c.Internal.MpoolGetNonce(ctx, addr)
}

func (c *Struct) ChainGetBlock(ctx context.Context, b cid.Cid) (*chain.BlockHeader, error) {
	return c.Internal.ChainGetBlock(ctx, b)
}

func (c *Struct) ChainGetBlockMessages(ctx context.Context, b cid.Cid) ([]*chain.SignedMessage, error) {
	return c.Internal.ChainGetBlockMessages(ctx, b)
}

var _ API = &Struct{}
