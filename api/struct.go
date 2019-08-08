package api

import (
	"context"

	"github.com/libp2p/go-libp2p-core/network"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
)

// All permissions are listed in permissioned.go
var _ = AllPermissions

type CommonStruct struct {
	Internal struct {
		AuthVerify func(ctx context.Context, token string) ([]string, error) `perm:"read"`
		AuthNew    func(ctx context.Context, perms []string) ([]byte, error) `perm:"admin"`

		NetConnectedness func(context.Context, peer.ID) (network.Connectedness, error) `perm:"read"`
		NetPeers         func(context.Context) ([]peer.AddrInfo, error)                `perm:"read"`
		NetConnect       func(context.Context, peer.AddrInfo) error                    `perm:"write"`
		NetAddrsListen   func(context.Context) (peer.AddrInfo, error)                  `perm:"read"`
		NetDisconnect    func(context.Context, peer.ID) error                          `perm:"write"`

		ID      func(context.Context) (peer.ID, error) `perm:"read"`
		Version func(context.Context) (Version, error) `perm:"read"`
	}
}

// FullNodeStruct implements API passing calls to user-provided function values.
type FullNodeStruct struct {
	CommonStruct

	Internal struct {
		ChainNotify           func(context.Context) (<-chan *store.HeadChange, error)                             `perm:"read"`
		ChainSubmitBlock      func(ctx context.Context, blk *chain.BlockMsg) error                                `perm:"write"`
		ChainHead             func(context.Context) (*types.TipSet, error)                                        `perm:"read"`
		ChainGetRandomness    func(context.Context, *types.TipSet) ([]byte, error)                                `perm:"read"`
		ChainWaitMsg          func(context.Context, cid.Cid) (*MsgWait, error)                                    `perm:"read"`
		ChainGetBlock         func(context.Context, cid.Cid) (*types.BlockHeader, error)                          `perm:"read"`
		ChainGetBlockMessages func(context.Context, cid.Cid) (*BlockMessages, error)                              `perm:"read"`
		ChainGetBlockReceipts func(context.Context, cid.Cid) ([]*types.MessageReceipt, error)                     `perm:"read"`
		ChainCall             func(context.Context, *types.Message, *types.TipSet) (*types.MessageReceipt, error) `perm:"read"`

		MpoolPending func(context.Context, *types.TipSet) ([]*types.SignedMessage, error) `perm:"read"`
		MpoolPush    func(context.Context, *types.SignedMessage) error                    `perm:"write"`

		MinerStart       func(context.Context, address.Address) error                                                                                                `perm:"admin"`
		MinerCreateBlock func(context.Context, address.Address, *types.TipSet, []types.Ticket, types.ElectionProof, []*types.SignedMessage) (*chain.BlockMsg, error) `perm:"write"`

		WalletNew            func(context.Context, string) (address.Address, error)                   `perm:"write"`
		WalletList           func(context.Context) ([]address.Address, error)                         `perm:"write"`
		WalletBalance        func(context.Context, address.Address) (types.BigInt, error)             `perm:"read"`
		WalletSign           func(context.Context, address.Address, []byte) (*types.Signature, error) `perm:"sign"`
		WalletDefaultAddress func(context.Context) (address.Address, error)                           `perm:"write"`
		MpoolGetNonce        func(context.Context, address.Address) (uint64, error)                   `perm:"read"`

		ClientImport      func(ctx context.Context, path string) (cid.Cid, error)                                                                     `perm:"write"`
		ClientListImports func(ctx context.Context) ([]Import, error)                                                                                 `perm:"read"`
		ClientStartDeal   func(ctx context.Context, data cid.Cid, miner address.Address, price types.BigInt, blocksDuration uint64) (*cid.Cid, error) `perm:"admin"`

		StateMinerSectors    func(context.Context, address.Address) ([]*SectorInfo, error) `perm:"read"`
		StateMinerProvingSet func(context.Context, address.Address) ([]*SectorInfo, error) `perm:"read"`
	}
}

type StorageMinerStruct struct {
	CommonStruct

	Internal struct {
		StoreGarbageData func(context.Context) (uint64, error) `perm:"write"`

		SectorsStatus     func(context.Context, uint64) (sectorbuilder.SectorSealingStatus, error) `perm:"read"`
		SectorsStagedList func(context.Context) ([]sectorbuilder.StagedSectorMetadata, error)      `perm:"read"`
		SectorsStagedSeal func(context.Context) error                                              `perm:"write"`
	}
}

func (c *CommonStruct) AuthVerify(ctx context.Context, token string) ([]string, error) {
	return c.Internal.AuthVerify(ctx, token)
}

func (c *CommonStruct) AuthNew(ctx context.Context, perms []string) ([]byte, error) {
	return c.Internal.AuthNew(ctx, perms)
}

func (c *CommonStruct) NetConnectedness(ctx context.Context, pid peer.ID) (network.Connectedness, error) {
	return c.Internal.NetConnectedness(ctx, pid)
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

func (c *CommonStruct) NetDisconnect(ctx context.Context, p peer.ID) error {
	return c.Internal.NetDisconnect(ctx, p)
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

func (c *FullNodeStruct) ClientStartDeal(ctx context.Context, data cid.Cid, miner address.Address, price types.BigInt, blocksDuration uint64) (*cid.Cid, error) {
	return c.Internal.ClientStartDeal(ctx, data, miner, price, blocksDuration)
}

func (c *FullNodeStruct) MpoolPending(ctx context.Context, ts *types.TipSet) ([]*types.SignedMessage, error) {
	return c.Internal.MpoolPending(ctx, ts)
}

func (c *FullNodeStruct) MpoolPush(ctx context.Context, smsg *types.SignedMessage) error {
	return c.Internal.MpoolPush(ctx, smsg)
}

func (c *FullNodeStruct) MinerStart(ctx context.Context, addr address.Address) error {
	return c.Internal.MinerStart(ctx, addr)
}

func (c *FullNodeStruct) MinerCreateBlock(ctx context.Context, addr address.Address, base *types.TipSet, tickets []types.Ticket, eproof types.ElectionProof, msgs []*types.SignedMessage) (*chain.BlockMsg, error) {
	return c.Internal.MinerCreateBlock(ctx, addr, base, tickets, eproof, msgs)
}

func (c *FullNodeStruct) ChainSubmitBlock(ctx context.Context, blk *chain.BlockMsg) error {
	return c.Internal.ChainSubmitBlock(ctx, blk)
}

func (c *FullNodeStruct) ChainHead(ctx context.Context) (*types.TipSet, error) {
	return c.Internal.ChainHead(ctx)
}

func (c *FullNodeStruct) ChainGetRandomness(ctx context.Context, pts *types.TipSet) ([]byte, error) {
	return c.Internal.ChainGetRandomness(ctx, pts)
}

func (c *FullNodeStruct) ChainWaitMsg(ctx context.Context, msgc cid.Cid) (*MsgWait, error) {
	return c.Internal.ChainWaitMsg(ctx, msgc)
}

func (c *FullNodeStruct) ChainCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	return c.Internal.ChainCall(ctx, msg, ts)
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

func (c *FullNodeStruct) ChainGetBlock(ctx context.Context, b cid.Cid) (*types.BlockHeader, error) {
	return c.Internal.ChainGetBlock(ctx, b)
}

func (c *FullNodeStruct) ChainGetBlockMessages(ctx context.Context, b cid.Cid) (*BlockMessages, error) {
	return c.Internal.ChainGetBlockMessages(ctx, b)
}

func (c *FullNodeStruct) ChainGetBlockReceipts(ctx context.Context, b cid.Cid) ([]*types.MessageReceipt, error) {
	return c.Internal.ChainGetBlockReceipts(ctx, b)
}

func (c *FullNodeStruct) ChainNotify(ctx context.Context) (<-chan *store.HeadChange, error) {
	return c.Internal.ChainNotify(ctx)
}

func (c *FullNodeStruct) StateMinerSectors(ctx context.Context, addr address.Address) ([]*SectorInfo, error) {
	return c.Internal.StateMinerSectors(ctx, addr)
}

func (c *FullNodeStruct) StateMinerProvingSet(ctx context.Context, addr address.Address) ([]*SectorInfo, error) {
	return c.Internal.StateMinerProvingSet(ctx, addr)
}

func (c *StorageMinerStruct) StoreGarbageData(ctx context.Context) (uint64, error) {
	return c.Internal.StoreGarbageData(ctx)
}

// Get the status of a given sector by ID
func (c *StorageMinerStruct) SectorsStatus(ctx context.Context, sid uint64) (sectorbuilder.SectorSealingStatus, error) {
	return c.Internal.SectorsStatus(ctx, sid)
}

// List all staged sectors
func (c *StorageMinerStruct) SectorsStagedList(ctx context.Context) ([]sectorbuilder.StagedSectorMetadata, error) {
	return c.Internal.SectorsStagedList(ctx)
}

// Seal all staged sectors
func (c *StorageMinerStruct) SectorsStagedSeal(ctx context.Context) error {
	return c.Internal.SectorsStagedSeal(ctx)
}

var _ Common = &CommonStruct{}
var _ FullNode = &FullNodeStruct{}
var _ StorageMiner = &StorageMinerStruct{}
