package api

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
)

// All permissions are listed in permissioned.go
var _ = AllPermissions

// Struct implements API passing calls to user-provided function values.
type Struct struct {
	Internal struct {
		AuthVerify func(ctx context.Context, token string) ([]string, error) `perm:"read"`
		AuthNew    func(ctx context.Context, perms []string) ([]byte, error) `perm:"admin"`

		ID      func(context.Context) (peer.ID, error) `perm:"read"`
		Version func(context.Context) (Version, error) `perm:"read"`

		ChainSubmitBlock      func(ctx context.Context, blk *chain.BlockMsg) error           `perm:"write"`
		ChainHead             func(context.Context) (*chain.TipSet, error)                   `perm:"read"`
		ChainGetRandomness    func(context.Context, *chain.TipSet) ([]byte, error)           `perm:"read"`
		ChainWaitMsg          func(context.Context, cid.Cid) (*MsgWait, error)               `perm:"read"`
		ChainGetBlock         func(context.Context, cid.Cid) (*chain.BlockHeader, error)     `perm:"read"`
		ChainGetBlockMessages func(context.Context, cid.Cid) ([]*chain.SignedMessage, error) `perm:"read"`

		MpoolPending func(context.Context, *chain.TipSet) ([]*chain.SignedMessage, error) `perm:"read"`
		MpoolPush    func(context.Context, *chain.SignedMessage) error                    `perm:"write"`

		MinerStart       func(context.Context, address.Address) error                                                                                                `perm:"admin"`
		MinerCreateBlock func(context.Context, address.Address, *chain.TipSet, []chain.Ticket, chain.ElectionProof, []*chain.SignedMessage) (*chain.BlockMsg, error) `perm:"write"`

		WalletNew            func(context.Context, string) (address.Address, error)                   `perm:"write"`
		WalletList           func(context.Context) ([]address.Address, error)                         `perm:"write"`
		WalletBalance        func(context.Context, address.Address) (types.BigInt, error)             `perm:"read"`
		WalletSign           func(context.Context, address.Address, []byte) (*chain.Signature, error) `perm:"sign"`
		WalletDefaultAddress func(context.Context) (address.Address, error)                           `perm:"write"`
		MpoolGetNonce        func(context.Context, address.Address) (uint64, error)                   `perm:"read"`

		ClientImport      func(ctx context.Context, path string) (cid.Cid, error) `perm:"write"`
		ClientListImports func(ctx context.Context) ([]Import, error)             `perm:"read"`

		NetPeers       func(context.Context) ([]peer.AddrInfo, error) `perm:"read"`
		NetConnect     func(context.Context, peer.AddrInfo) error     `perm:"write"`
		NetAddrsListen func(context.Context) (peer.AddrInfo, error)   `perm:"read"`
	}
}

func (c *Struct) AuthVerify(ctx context.Context, token string) ([]string, error) {
	return c.Internal.AuthVerify(ctx, token)
}

func (c *Struct) AuthNew(ctx context.Context, perms []string) ([]byte, error) {
	return c.Internal.AuthNew(ctx, perms)
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
