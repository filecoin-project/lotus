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

type CommonStruct struct {
	Internal struct {
		AuthVerify func(ctx context.Context, token string) ([]string, error) `perm:"read"`
		AuthNew    func(ctx context.Context, perms []string) ([]byte, error) `perm:"admin"`

		NetPeers       func(context.Context) ([]peer.AddrInfo, error) `perm:"read"`
		NetConnect     func(context.Context, peer.AddrInfo) error     `perm:"write"`
		NetAddrsListen func(context.Context) (peer.AddrInfo, error)   `perm:"read"`

		ID      func(context.Context) (peer.ID, error) `perm:"read"`
		Version func(context.Context) (Version, error) `perm:"read"`
	}
}

// FullNodeStruct implements API passing calls to user-provided function values.
type FullNodeStruct struct {
	CommonStruct

	Internal struct {
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
		WalletSign           func(context.Context, address.Address, []byte) (*types.Signature, error) `perm:"sign"`
		WalletDefaultAddress func(context.Context) (address.Address, error)                           `perm:"write"`
		MpoolGetNonce        func(context.Context, address.Address) (uint64, error)                   `perm:"read"`

		ClientImport      func(ctx context.Context, path string) (cid.Cid, error) `perm:"write"`
		ClientListImports func(ctx context.Context) ([]Import, error)             `perm:"read"`
	}
}

type StorageMinerStruct struct {
	CommonStruct

	Internal struct {
	}
}

func (c *CommonStruct) AuthVerify(ctx context.Context, token string) ([]string, error) {
	return c.Internal.AuthVerify(ctx, token)
}

func (c *CommonStruct) AuthNew(ctx context.Context, perms []string) ([]byte, error) {
	return c.Internal.AuthNew(ctx, perms)
}

func (c *CommonStruct) NetPeers(ctx context.Context) ([]peer.AddrInfo, error) {
	return c.Internal.NetPeers(ctx)
}

func (c *CommonStruct) NetConnect(ctx context.Context, p peer.AddrInfo) error {
	return c.Internal.NetConnect(ctx, p)
}

func (c *CommonStruct) NetAddrsListen(ctx context.Context) (peer.AddrInfo, error) {
	return c.Internal.NetAddrsListen(ctx)
}

// ID implements API.ID
func (c *CommonStruct) ID(ctx context.Context) (peer.ID, error) {
	return c.Internal.ID(ctx)
}

// Version implements API.Version
func (c *CommonStruct) Version(ctx context.Context) (Version, error) {
	return c.Internal.Version(ctx)
}

func (c *FullNodeStruct) ClientListImports(ctx context.Context) ([]Import, error) {
	return c.Internal.ClientListImports(ctx)
}

func (c *FullNodeStruct) ClientImport(ctx context.Context, path string) (cid.Cid, error) {
	return c.Internal.ClientImport(ctx, path)
}

func (c *FullNodeStruct) MpoolPending(ctx context.Context, ts *chain.TipSet) ([]*chain.SignedMessage, error) {
	return c.Internal.MpoolPending(ctx, ts)
}

func (c *FullNodeStruct) MpoolPush(ctx context.Context, smsg *chain.SignedMessage) error {
	return c.Internal.MpoolPush(ctx, smsg)
}

func (c *FullNodeStruct) MinerStart(ctx context.Context, addr address.Address) error {
	return c.Internal.MinerStart(ctx, addr)
}

func (c *FullNodeStruct) MinerCreateBlock(ctx context.Context, addr address.Address, base *chain.TipSet, tickets []chain.Ticket, eproof chain.ElectionProof, msgs []*chain.SignedMessage) (*chain.BlockMsg, error) {
	return c.Internal.MinerCreateBlock(ctx, addr, base, tickets, eproof, msgs)
}

func (c *FullNodeStruct) ChainSubmitBlock(ctx context.Context, blk *chain.BlockMsg) error {
	return c.Internal.ChainSubmitBlock(ctx, blk)
}

func (c *FullNodeStruct) ChainHead(ctx context.Context) (*chain.TipSet, error) {
	return c.Internal.ChainHead(ctx)
}

func (c *FullNodeStruct) ChainGetRandomness(ctx context.Context, pts *chain.TipSet) ([]byte, error) {
	return c.Internal.ChainGetRandomness(ctx, pts)
}

func (c *FullNodeStruct) ChainWaitMsg(ctx context.Context, msgc cid.Cid) (*MsgWait, error) {
	return c.Internal.ChainWaitMsg(ctx, msgc)
}

func (c *FullNodeStruct) WalletNew(ctx context.Context, typ string) (address.Address, error) {
	return c.Internal.WalletNew(ctx, typ)
}

func (c *FullNodeStruct) WalletList(ctx context.Context) ([]address.Address, error) {
	return c.Internal.WalletList(ctx)
}

func (c *FullNodeStruct) WalletBalance(ctx context.Context, a address.Address) (types.BigInt, error) {
	return c.Internal.WalletBalance(ctx, a)
}

func (c *FullNodeStruct) WalletSign(ctx context.Context, k address.Address, msg []byte) (*types.Signature, error) {
	return c.Internal.WalletSign(ctx, k, msg)
}

func (c *FullNodeStruct) WalletDefaultAddress(ctx context.Context) (address.Address, error) {
	return c.Internal.WalletDefaultAddress(ctx)
}

func (c *FullNodeStruct) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return c.Internal.MpoolGetNonce(ctx, addr)
}

func (c *FullNodeStruct) ChainGetBlock(ctx context.Context, b cid.Cid) (*chain.BlockHeader, error) {
	return c.Internal.ChainGetBlock(ctx, b)
}

func (c *FullNodeStruct) ChainGetBlockMessages(ctx context.Context, b cid.Cid) ([]*chain.SignedMessage, error) {
	return c.Internal.ChainGetBlockMessages(ctx, b)
}

var _ Common = &CommonStruct{}
var _ FullNode = &FullNodeStruct{}
var _ StorageMiner = &StorageMinerStruct{}
