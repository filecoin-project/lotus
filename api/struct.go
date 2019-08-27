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
		ChainGetActor         func(context.Context, address.Address, *types.TipSet) (*types.Actor, error)         `perm:"read"`
		ChainReadState        func(context.Context, *types.Actor, *types.TipSet) (*ActorState, error)             `perm:"read"`

		MpoolPending func(context.Context, *types.TipSet) ([]*types.SignedMessage, error) `perm:"read"`
		MpoolPush    func(context.Context, *types.SignedMessage) error                    `perm:"write"`

		MinerRegister    func(context.Context, address.Address) error                                                                                                         `perm:"admin"`
		MinerUnregister  func(context.Context, address.Address) error                                                                                                         `perm:"admin"`
		MinerAddresses   func(context.Context) ([]address.Address, error)                                                                                                     `perm:"write"`
		MinerCreateBlock func(context.Context, address.Address, *types.TipSet, []*types.Ticket, types.ElectionProof, []*types.SignedMessage, uint64) (*chain.BlockMsg, error) `perm:"write"`

		WalletNew            func(context.Context, string) (address.Address, error)                               `perm:"write"`
		WalletHas            func(context.Context, address.Address) (bool, error)                                 `perm:"write"`
		WalletList           func(context.Context) ([]address.Address, error)                                     `perm:"write"`
		WalletBalance        func(context.Context, address.Address) (types.BigInt, error)                         `perm:"read"`
		WalletSign           func(context.Context, address.Address, []byte) (*types.Signature, error)             `perm:"sign"`
		WalletSignMessage    func(context.Context, address.Address, *types.Message) (*types.SignedMessage, error) `perm:"sign"`
		WalletDefaultAddress func(context.Context) (address.Address, error)                                       `perm:"write"`
		MpoolGetNonce        func(context.Context, address.Address) (uint64, error)                               `perm:"read"`

		ClientImport      func(ctx context.Context, path string) (cid.Cid, error)                                                                     `perm:"write"`
		ClientListImports func(ctx context.Context) ([]Import, error)                                                                                 `perm:"write"`
		ClientHasLocal    func(ctx context.Context, root cid.Cid) (bool, error)                                                                       `perm:"write"`
		ClientFindData    func(ctx context.Context, root cid.Cid) ([]QueryOffer, error)                                                               `perm:"read"`
		ClientStartDeal   func(ctx context.Context, data cid.Cid, miner address.Address, price types.BigInt, blocksDuration uint64) (*cid.Cid, error) `perm:"admin"`
		ClientRetrieve    func(ctx context.Context, order RetrievalOrder, path string) error                                                          `perm:"admin"`

		StateMinerSectors    func(context.Context, address.Address) ([]*SectorInfo, error)             `perm:"read"`
		StateMinerProvingSet func(context.Context, address.Address) ([]*SectorInfo, error)             `perm:"read"`
		StateMinerPower      func(context.Context, address.Address, *types.TipSet) (MinerPower, error) `perm:"read"`

		PaychCreate                func(ctx context.Context, from, to address.Address, amt types.BigInt) (address.Address, error) `perm:"sign"`
		PaychList                  func(context.Context) ([]address.Address, error)                                               `perm:"read"`
		PaychStatus                func(context.Context, address.Address) (*PaychStatus, error)                                   `perm:"read"`
		PaychClose                 func(context.Context, address.Address) (cid.Cid, error)                                        `perm:"sign"`
		PaychVoucherCheck          func(context.Context, *types.SignedVoucher) error                                              `perm:"read"`
		PaychVoucherCheckValid     func(context.Context, address.Address, *types.SignedVoucher) error                             `perm:"read"`
		PaychVoucherCheckSpendable func(context.Context, address.Address, *types.SignedVoucher, []byte, []byte) (bool, error)     `perm:"read"`
		PaychVoucherAdd            func(context.Context, address.Address, *types.SignedVoucher) error                             `perm:"write"`
		PaychVoucherCreate         func(context.Context, address.Address, types.BigInt, uint64) (*types.SignedVoucher, error)     `perm:"sign"`
		PaychVoucherList           func(context.Context, address.Address) ([]*types.SignedVoucher, error)                         `perm:"write"`
		PaychVoucherSubmit         func(context.Context, address.Address, *types.SignedVoucher) (cid.Cid, error)                  `perm:"sign"`
	}
}

type StorageMinerStruct struct {
	CommonStruct

	Internal struct {
		ActorAddresses func(context.Context) ([]address.Address, error) `perm:"read"`

		StoreGarbageData func(context.Context) (uint64, error) `perm:"write"`

		SectorsStatus     func(context.Context, uint64) (sectorbuilder.SectorSealingStatus, error) `perm:"read"`
		SectorsStagedList func(context.Context) ([]sectorbuilder.StagedSectorMetadata, error)      `perm:"read"`
		SectorsStagedSeal func(context.Context) error                                              `perm:"write"`

		SectorsRefs func(context.Context) (map[string][]SealedRef, error) `perm:"read"`
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

func (c *FullNodeStruct) ClientHasLocal(ctx context.Context, root cid.Cid) (bool, error) {
	return c.Internal.ClientHasLocal(ctx, root)
}

func (c *FullNodeStruct) ClientFindData(ctx context.Context, root cid.Cid) ([]QueryOffer, error) {
	return c.Internal.ClientFindData(ctx, root)
}

func (c *FullNodeStruct) ClientStartDeal(ctx context.Context, data cid.Cid, miner address.Address, price types.BigInt, blocksDuration uint64) (*cid.Cid, error) {
	return c.Internal.ClientStartDeal(ctx, data, miner, price, blocksDuration)
}

func (c *FullNodeStruct) ClientRetrieve(ctx context.Context, order RetrievalOrder, path string) error {
	return c.Internal.ClientRetrieve(ctx, order, path)
}

func (c *FullNodeStruct) MpoolPending(ctx context.Context, ts *types.TipSet) ([]*types.SignedMessage, error) {
	return c.Internal.MpoolPending(ctx, ts)
}

func (c *FullNodeStruct) MpoolPush(ctx context.Context, smsg *types.SignedMessage) error {
	return c.Internal.MpoolPush(ctx, smsg)
}

func (c *FullNodeStruct) MinerRegister(ctx context.Context, addr address.Address) error {
	return c.Internal.MinerRegister(ctx, addr)
}

func (c *FullNodeStruct) MinerUnregister(ctx context.Context, addr address.Address) error {
	return c.Internal.MinerUnregister(ctx, addr)
}

func (c *FullNodeStruct) MinerAddresses(ctx context.Context) ([]address.Address, error) {
	return c.Internal.MinerAddresses(ctx)
}

func (c *FullNodeStruct) MinerCreateBlock(ctx context.Context, addr address.Address, base *types.TipSet, tickets []*types.Ticket, eproof types.ElectionProof, msgs []*types.SignedMessage, ts uint64) (*chain.BlockMsg, error) {
	return c.Internal.MinerCreateBlock(ctx, addr, base, tickets, eproof, msgs, ts)
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

func (c *FullNodeStruct) ChainGetActor(ctx context.Context, actor address.Address, ts *types.TipSet) (*types.Actor, error) {
	return c.Internal.ChainGetActor(ctx, actor, ts)
}

func (c *FullNodeStruct) ChainReadState(ctx context.Context, act *types.Actor, ts *types.TipSet) (*ActorState, error) {
	return c.Internal.ChainReadState(ctx, act, ts)
}

func (c *FullNodeStruct) WalletNew(ctx context.Context, typ string) (address.Address, error) {
	return c.Internal.WalletNew(ctx, typ)
}

func (c *FullNodeStruct) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	return c.Internal.WalletHas(ctx, addr)
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

func (c *FullNodeStruct) WalletSignMessage(ctx context.Context, k address.Address, msg *types.Message) (*types.SignedMessage, error) {
	return c.Internal.WalletSignMessage(ctx, k, msg)
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

func (c *FullNodeStruct) StateMinerPower(ctx context.Context, a address.Address, ts *types.TipSet) (MinerPower, error) {
	return c.Internal.StateMinerPower(ctx, a, ts)
}

func (c *FullNodeStruct) PaychCreate(ctx context.Context, from, to address.Address, amt types.BigInt) (address.Address, error) {
	return c.Internal.PaychCreate(ctx, from, to, amt)
}

func (c *FullNodeStruct) PaychList(ctx context.Context) ([]address.Address, error) {
	return c.Internal.PaychList(ctx)
}

func (c *FullNodeStruct) PaychStatus(ctx context.Context, pch address.Address) (*PaychStatus, error) {
	return c.Internal.PaychStatus(ctx, pch)
}

func (c *FullNodeStruct) PaychVoucherCheckValid(ctx context.Context, addr address.Address, sv *types.SignedVoucher) error {
	return c.Internal.PaychVoucherCheckValid(ctx, addr, sv)
}

func (c *FullNodeStruct) PaychVoucherCheckSpendable(ctx context.Context, addr address.Address, sv *types.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	return c.Internal.PaychVoucherCheckSpendable(ctx, addr, sv, secret, proof)
}

func (c *FullNodeStruct) PaychVoucherAdd(ctx context.Context, addr address.Address, sv *types.SignedVoucher) error {
	return c.Internal.PaychVoucherAdd(ctx, addr, sv)
}

func (c *FullNodeStruct) PaychVoucherCreate(ctx context.Context, pch address.Address, amt types.BigInt, lane uint64) (*types.SignedVoucher, error) {
	return c.Internal.PaychVoucherCreate(ctx, pch, amt, lane)
}

func (c *FullNodeStruct) PaychVoucherList(ctx context.Context, pch address.Address) ([]*types.SignedVoucher, error) {
	return c.Internal.PaychVoucherList(ctx, pch)
}

func (c *FullNodeStruct) PaychClose(ctx context.Context, a address.Address) (cid.Cid, error) {
	return c.Internal.PaychClose(ctx, a)
}

func (c *FullNodeStruct) PaychVoucherSubmit(ctx context.Context, ch address.Address, sv *types.SignedVoucher) (cid.Cid, error) {
	return c.Internal.PaychVoucherSubmit(ctx, ch, sv)
}

func (c *StorageMinerStruct) ActorAddresses(ctx context.Context) ([]address.Address, error) {
	return c.Internal.ActorAddresses(ctx)
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

func (c *StorageMinerStruct) SectorsRefs(ctx context.Context) (map[string][]SealedRef, error) {
	return c.Internal.SectorsRefs(ctx)
}

var _ Common = &CommonStruct{}
var _ FullNode = &FullNodeStruct{}
var _ StorageMiner = &StorageMinerStruct{}
