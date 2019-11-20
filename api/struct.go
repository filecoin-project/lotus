package api

import (
	"context"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

// All permissions are listed in permissioned.go
var _ = AllPermissions

type CommonStruct struct {
	Internal struct {
		AuthVerify func(ctx context.Context, token string) ([]Permission, error) `perm:"read"`
		AuthNew    func(ctx context.Context, perms []Permission) ([]byte, error) `perm:"admin"`

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
		ChainNotify            func(context.Context) (<-chan []*store.HeadChange, error)                    `perm:"read"`
		ChainHead              func(context.Context) (*types.TipSet, error)                                 `perm:"read"`
		ChainGetRandomness     func(context.Context, types.TipSetKey, []*types.Ticket, int) ([]byte, error) `perm:"read"`
		ChainGetBlock          func(context.Context, cid.Cid) (*types.BlockHeader, error)                   `perm:"read"`
		ChainGetTipSet         func(context.Context, types.TipSetKey) (*types.TipSet, error)                `perm:"read"`
		ChainGetBlockMessages  func(context.Context, cid.Cid) (*BlockMessages, error)                       `perm:"read"`
		ChainGetParentReceipts func(context.Context, cid.Cid) ([]*types.MessageReceipt, error)              `perm:"read"`
		ChainGetParentMessages func(context.Context, cid.Cid) ([]Message, error)                            `perm:"read"`
		ChainGetTipSetByHeight func(context.Context, uint64, *types.TipSet) (*types.TipSet, error)          `perm:"read"`
		ChainReadObj           func(context.Context, cid.Cid) ([]byte, error)                               `perm:"read"`
		ChainSetHead           func(context.Context, *types.TipSet) error                                   `perm:"admin"`
		ChainGetGenesis        func(context.Context) (*types.TipSet, error)                                 `perm:"read"`
		ChainTipSetWeight      func(context.Context, *types.TipSet) (types.BigInt, error)                   `perm:"read"`

		SyncState          func(context.Context) (*SyncState, error)                    `perm:"read"`
		SyncSubmitBlock    func(ctx context.Context, blk *types.BlockMsg) error         `perm:"write"`
		SyncIncomingBlocks func(ctx context.Context) (<-chan *types.BlockHeader, error) `perm:"read"`

		MpoolPending     func(context.Context, *types.TipSet) ([]*types.SignedMessage, error) `perm:"read"`
		MpoolPush        func(context.Context, *types.SignedMessage) error                    `perm:"write"`
		MpoolPushMessage func(context.Context, *types.Message) (*types.SignedMessage, error)  `perm:"sign"`
		MpoolGetNonce    func(context.Context, address.Address) (uint64, error)               `perm:"read"`
		MpoolSub         func(context.Context) (<-chan MpoolUpdate, error)                    `perm:"read"`

		MinerRegister    func(context.Context, address.Address) error                                                                                                         `perm:"admin"`
		MinerUnregister  func(context.Context, address.Address) error                                                                                                         `perm:"admin"`
		MinerAddresses   func(context.Context) ([]address.Address, error)                                                                                                     `perm:"write"`
		MinerCreateBlock func(context.Context, address.Address, *types.TipSet, []*types.Ticket, types.ElectionProof, []*types.SignedMessage, uint64) (*types.BlockMsg, error) `perm:"write"`

		WalletNew            func(context.Context, string) (address.Address, error)                               `perm:"write"`
		WalletHas            func(context.Context, address.Address) (bool, error)                                 `perm:"write"`
		WalletList           func(context.Context) ([]address.Address, error)                                     `perm:"write"`
		WalletBalance        func(context.Context, address.Address) (types.BigInt, error)                         `perm:"read"`
		WalletSign           func(context.Context, address.Address, []byte) (*types.Signature, error)             `perm:"sign"`
		WalletSignMessage    func(context.Context, address.Address, *types.Message) (*types.SignedMessage, error) `perm:"sign"`
		WalletDefaultAddress func(context.Context) (address.Address, error)                                       `perm:"write"`
		WalletSetDefault     func(context.Context, address.Address) error                                         `perm:"admin"`
		WalletExport         func(context.Context, address.Address) (*types.KeyInfo, error)                       `perm:"admin"`
		WalletImport         func(context.Context, *types.KeyInfo) (address.Address, error)                       `perm:"admin"`

		ClientImport      func(ctx context.Context, path string) (cid.Cid, error)                                                                     `perm:"admin"`
		ClientListImports func(ctx context.Context) ([]Import, error)                                                                                 `perm:"write"`
		ClientHasLocal    func(ctx context.Context, root cid.Cid) (bool, error)                                                                       `perm:"write"`
		ClientFindData    func(ctx context.Context, root cid.Cid) ([]QueryOffer, error)                                                               `perm:"read"`
		ClientStartDeal   func(ctx context.Context, data cid.Cid, miner address.Address, price types.BigInt, blocksDuration uint64) (*cid.Cid, error) `perm:"admin"`
		ClientGetDealInfo func(context.Context, cid.Cid) (*DealInfo, error)                                                                           `perm:"read"`
		ClientListDeals   func(ctx context.Context) ([]DealInfo, error)                                                                               `perm:"write"`
		ClientRetrieve    func(ctx context.Context, order RetrievalOrder, path string) error                                                          `perm:"admin"`
		ClientQueryAsk    func(ctx context.Context, p peer.ID, miner address.Address) (*types.SignedStorageAsk, error)                                `perm:"read"`

		StateMinerSectors          func(context.Context, address.Address, *types.TipSet) ([]*ChainSectorInfo, error)               `perm:"read"`
		StateMinerProvingSet       func(context.Context, address.Address, *types.TipSet) ([]*ChainSectorInfo, error)               `perm:"read"`
		StateMinerPower            func(context.Context, address.Address, *types.TipSet) (MinerPower, error)                       `perm:"read"`
		StateMinerWorker           func(context.Context, address.Address, *types.TipSet) (address.Address, error)                  `perm:"read"`
		StateMinerPeerID           func(ctx context.Context, m address.Address, ts *types.TipSet) (peer.ID, error)                 `perm:"read"`
		StateMinerProvingPeriodEnd func(ctx context.Context, actor address.Address, ts *types.TipSet) (uint64, error)              `perm:"read"`
		StateMinerSectorSize       func(context.Context, address.Address, *types.TipSet) (uint64, error)                           `perm:"read"`
		StateCall                  func(context.Context, *types.Message, *types.TipSet) (*types.MessageReceipt, error)             `perm:"read"`
		StateReplay                func(context.Context, *types.TipSet, cid.Cid) (*ReplayResults, error)                           `perm:"read"`
		StateGetActor              func(context.Context, address.Address, *types.TipSet) (*types.Actor, error)                     `perm:"read"`
		StateReadState             func(context.Context, *types.Actor, *types.TipSet) (*ActorState, error)                         `perm:"read"`
		StatePledgeCollateral      func(context.Context, *types.TipSet) (types.BigInt, error)                                      `perm:"read"`
		StateWaitMsg               func(context.Context, cid.Cid) (*MsgWait, error)                                                `perm:"read"`
		StateListMiners            func(context.Context, *types.TipSet) ([]address.Address, error)                                 `perm:"read"`
		StateListActors            func(context.Context, *types.TipSet) ([]address.Address, error)                                 `perm:"read"`
		StateMarketBalance         func(context.Context, address.Address, *types.TipSet) (actors.StorageParticipantBalance, error) `perm:"read"`
		StateMarketParticipants    func(context.Context, *types.TipSet) (map[string]actors.StorageParticipantBalance, error)       `perm:"read"`
		StateMarketDeals           func(context.Context, *types.TipSet) (map[string]actors.OnChainDeal, error)                     `perm:"read"`
		StateMarketStorageDeal     func(context.Context, uint64, *types.TipSet) (*actors.OnChainDeal, error)                       `perm:"read"`
		StateLookupID              func(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error)      `perm:"read"`
		StateChangedActors         func(context.Context, cid.Cid, cid.Cid) (map[string]types.Actor, error)                         `perm:"read"`
		StateGetReceipt            func(context.Context, cid.Cid, *types.TipSet) (*types.MessageReceipt, error)                    `perm:"read"`

		MarketEnsureAvailable func(context.Context, address.Address, types.BigInt) error `perm:"sign"`

		PaychGet                   func(ctx context.Context, from, to address.Address, ensureFunds types.BigInt) (*ChannelInfo, error)      `perm:"sign"`
		PaychList                  func(context.Context) ([]address.Address, error)                                                         `perm:"read"`
		PaychStatus                func(context.Context, address.Address) (*PaychStatus, error)                                             `perm:"read"`
		PaychClose                 func(context.Context, address.Address) (cid.Cid, error)                                                  `perm:"sign"`
		PaychAllocateLane          func(context.Context, address.Address) (uint64, error)                                                   `perm:"sign"`
		PaychNewPayment            func(ctx context.Context, from, to address.Address, vouchers []VoucherSpec) (*PaymentInfo, error)        `perm:"sign"`
		PaychVoucherCheck          func(context.Context, *types.SignedVoucher) error                                                        `perm:"read"`
		PaychVoucherCheckValid     func(context.Context, address.Address, *types.SignedVoucher) error                                       `perm:"read"`
		PaychVoucherCheckSpendable func(context.Context, address.Address, *types.SignedVoucher, []byte, []byte) (bool, error)               `perm:"read"`
		PaychVoucherAdd            func(context.Context, address.Address, *types.SignedVoucher, []byte, types.BigInt) (types.BigInt, error) `perm:"write"`
		PaychVoucherCreate         func(context.Context, address.Address, types.BigInt, uint64) (*types.SignedVoucher, error)               `perm:"sign"`
		PaychVoucherList           func(context.Context, address.Address) ([]*types.SignedVoucher, error)                                   `perm:"write"`
		PaychVoucherSubmit         func(context.Context, address.Address, *types.SignedVoucher) (cid.Cid, error)                            `perm:"sign"`
	}
}

type StorageMinerStruct struct {
	CommonStruct

	Internal struct {
		ActorAddress func(context.Context) (address.Address, error) `perm:"read"`

		StoreGarbageData func(context.Context) error `perm:"write"`

		SectorsStatus func(context.Context, uint64) (SectorInfo, error)     `perm:"read"`
		SectorsList   func(context.Context) ([]uint64, error)               `perm:"read"`
		SectorsRefs   func(context.Context) (map[string][]SealedRef, error) `perm:"read"`

		WorkerStats func(context.Context) (WorkerStats, error) `perm:"read"`
	}
}

func (c *CommonStruct) AuthVerify(ctx context.Context, token string) ([]Permission, error) {
	return c.Internal.AuthVerify(ctx, token)
}

func (c *CommonStruct) AuthNew(ctx context.Context, perms []Permission) ([]byte, error) {
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
func (c *FullNodeStruct) ClientGetDealInfo(ctx context.Context, deal cid.Cid) (*DealInfo, error) {
	return c.Internal.ClientGetDealInfo(ctx, deal)
}

func (c *FullNodeStruct) ClientListDeals(ctx context.Context) ([]DealInfo, error) {
	return c.Internal.ClientListDeals(ctx)
}

func (c *FullNodeStruct) ClientRetrieve(ctx context.Context, order RetrievalOrder, path string) error {
	return c.Internal.ClientRetrieve(ctx, order, path)
}

func (c *FullNodeStruct) ClientQueryAsk(ctx context.Context, p peer.ID, miner address.Address) (*types.SignedStorageAsk, error) {
	return c.Internal.ClientQueryAsk(ctx, p, miner)
}

func (c *FullNodeStruct) MpoolPending(ctx context.Context, ts *types.TipSet) ([]*types.SignedMessage, error) {
	return c.Internal.MpoolPending(ctx, ts)
}

func (c *FullNodeStruct) MpoolPush(ctx context.Context, smsg *types.SignedMessage) error {
	return c.Internal.MpoolPush(ctx, smsg)
}

func (c *FullNodeStruct) MpoolPushMessage(ctx context.Context, msg *types.Message) (*types.SignedMessage, error) {
	return c.Internal.MpoolPushMessage(ctx, msg)
}

func (c *FullNodeStruct) MpoolSub(ctx context.Context) (<-chan MpoolUpdate, error) {
	return c.Internal.MpoolSub(ctx)
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

func (c *FullNodeStruct) MinerCreateBlock(ctx context.Context, addr address.Address, base *types.TipSet, tickets []*types.Ticket, eproof types.ElectionProof, msgs []*types.SignedMessage, ts uint64) (*types.BlockMsg, error) {
	return c.Internal.MinerCreateBlock(ctx, addr, base, tickets, eproof, msgs, ts)
}

func (c *FullNodeStruct) ChainHead(ctx context.Context) (*types.TipSet, error) {
	return c.Internal.ChainHead(ctx)
}

func (c *FullNodeStruct) ChainGetRandomness(ctx context.Context, pts types.TipSetKey, ticks []*types.Ticket, lb int) ([]byte, error) {
	return c.Internal.ChainGetRandomness(ctx, pts, ticks, lb)
}

func (c *FullNodeStruct) ChainGetTipSetByHeight(ctx context.Context, h uint64, ts *types.TipSet) (*types.TipSet, error) {
	return c.Internal.ChainGetTipSetByHeight(ctx, h, ts)
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

func (c *FullNodeStruct) WalletSetDefault(ctx context.Context, a address.Address) error {
	return c.Internal.WalletSetDefault(ctx, a)
}

func (c *FullNodeStruct) WalletExport(ctx context.Context, a address.Address) (*types.KeyInfo, error) {
	return c.Internal.WalletExport(ctx, a)
}

func (c *FullNodeStruct) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	return c.Internal.WalletImport(ctx, ki)
}

func (c *FullNodeStruct) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return c.Internal.MpoolGetNonce(ctx, addr)
}

func (c *FullNodeStruct) ChainGetBlock(ctx context.Context, b cid.Cid) (*types.BlockHeader, error) {
	return c.Internal.ChainGetBlock(ctx, b)
}

func (c *FullNodeStruct) ChainGetTipSet(ctx context.Context, key types.TipSetKey) (*types.TipSet, error) {
	return c.Internal.ChainGetTipSet(ctx, key)
}

func (c *FullNodeStruct) ChainGetBlockMessages(ctx context.Context, b cid.Cid) (*BlockMessages, error) {
	return c.Internal.ChainGetBlockMessages(ctx, b)
}

func (c *FullNodeStruct) ChainGetParentReceipts(ctx context.Context, b cid.Cid) ([]*types.MessageReceipt, error) {
	return c.Internal.ChainGetParentReceipts(ctx, b)
}

func (c *FullNodeStruct) ChainGetParentMessages(ctx context.Context, b cid.Cid) ([]Message, error) {
	return c.Internal.ChainGetParentMessages(ctx, b)
}

func (c *FullNodeStruct) ChainNotify(ctx context.Context) (<-chan []*store.HeadChange, error) {
	return c.Internal.ChainNotify(ctx)
}

func (c *FullNodeStruct) ChainReadObj(ctx context.Context, obj cid.Cid) ([]byte, error) {
	return c.Internal.ChainReadObj(ctx, obj)
}

func (c *FullNodeStruct) ChainSetHead(ctx context.Context, ts *types.TipSet) error {
	return c.Internal.ChainSetHead(ctx, ts)
}

func (c *FullNodeStruct) ChainGetGenesis(ctx context.Context) (*types.TipSet, error) {
	return c.Internal.ChainGetGenesis(ctx)
}

func (c *FullNodeStruct) ChainTipSetWeight(ctx context.Context, ts *types.TipSet) (types.BigInt, error) {
	return c.Internal.ChainTipSetWeight(ctx, ts)
}

func (c *FullNodeStruct) SyncState(ctx context.Context) (*SyncState, error) {
	return c.Internal.SyncState(ctx)
}

func (c *FullNodeStruct) SyncSubmitBlock(ctx context.Context, blk *types.BlockMsg) error {
	return c.Internal.SyncSubmitBlock(ctx, blk)
}

func (c *FullNodeStruct) SyncIncomingBlocks(ctx context.Context) (<-chan *types.BlockHeader, error) {
	return c.Internal.SyncIncomingBlocks(ctx)
}

func (c *FullNodeStruct) StateMinerSectors(ctx context.Context, addr address.Address, ts *types.TipSet) ([]*ChainSectorInfo, error) {
	return c.Internal.StateMinerSectors(ctx, addr, ts)
}

func (c *FullNodeStruct) StateMinerProvingSet(ctx context.Context, addr address.Address, ts *types.TipSet) ([]*ChainSectorInfo, error) {
	return c.Internal.StateMinerProvingSet(ctx, addr, ts)
}

func (c *FullNodeStruct) StateMinerPower(ctx context.Context, a address.Address, ts *types.TipSet) (MinerPower, error) {
	return c.Internal.StateMinerPower(ctx, a, ts)
}

func (c *FullNodeStruct) StateMinerWorker(ctx context.Context, m address.Address, ts *types.TipSet) (address.Address, error) {
	return c.Internal.StateMinerWorker(ctx, m, ts)
}

func (c *FullNodeStruct) StateMinerPeerID(ctx context.Context, m address.Address, ts *types.TipSet) (peer.ID, error) {
	return c.Internal.StateMinerPeerID(ctx, m, ts)
}

func (c *FullNodeStruct) StateMinerProvingPeriodEnd(ctx context.Context, actor address.Address, ts *types.TipSet) (uint64, error) {
	return c.Internal.StateMinerProvingPeriodEnd(ctx, actor, ts)
}

func (c *FullNodeStruct) StateMinerSectorSize(ctx context.Context, actor address.Address, ts *types.TipSet) (uint64, error) {
	return c.Internal.StateMinerSectorSize(ctx, actor, ts)
}

func (c *FullNodeStruct) StateCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	return c.Internal.StateCall(ctx, msg, ts)
}

func (c *FullNodeStruct) StateReplay(ctx context.Context, ts *types.TipSet, mc cid.Cid) (*ReplayResults, error) {
	return c.Internal.StateReplay(ctx, ts, mc)
}

func (c *FullNodeStruct) StateGetActor(ctx context.Context, actor address.Address, ts *types.TipSet) (*types.Actor, error) {
	return c.Internal.StateGetActor(ctx, actor, ts)
}

func (c *FullNodeStruct) StateReadState(ctx context.Context, act *types.Actor, ts *types.TipSet) (*ActorState, error) {
	return c.Internal.StateReadState(ctx, act, ts)
}

func (c *FullNodeStruct) StatePledgeCollateral(ctx context.Context, ts *types.TipSet) (types.BigInt, error) {
	return c.Internal.StatePledgeCollateral(ctx, ts)
}

func (c *FullNodeStruct) StateWaitMsg(ctx context.Context, msgc cid.Cid) (*MsgWait, error) {
	return c.Internal.StateWaitMsg(ctx, msgc)
}
func (c *FullNodeStruct) StateListMiners(ctx context.Context, ts *types.TipSet) ([]address.Address, error) {
	return c.Internal.StateListMiners(ctx, ts)
}

func (c *FullNodeStruct) StateListActors(ctx context.Context, ts *types.TipSet) ([]address.Address, error) {
	return c.Internal.StateListActors(ctx, ts)
}

func (c *FullNodeStruct) StateMarketBalance(ctx context.Context, addr address.Address, ts *types.TipSet) (actors.StorageParticipantBalance, error) {
	return c.Internal.StateMarketBalance(ctx, addr, ts)
}

func (c *FullNodeStruct) StateMarketParticipants(ctx context.Context, ts *types.TipSet) (map[string]actors.StorageParticipantBalance, error) {
	return c.Internal.StateMarketParticipants(ctx, ts)
}

func (c *FullNodeStruct) StateMarketDeals(ctx context.Context, ts *types.TipSet) (map[string]actors.OnChainDeal, error) {
	return c.Internal.StateMarketDeals(ctx, ts)
}

func (c *FullNodeStruct) StateMarketStorageDeal(ctx context.Context, dealid uint64, ts *types.TipSet) (*actors.OnChainDeal, error) {
	return c.Internal.StateMarketStorageDeal(ctx, dealid, ts)
}

func (c *FullNodeStruct) StateLookupID(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	return c.Internal.StateLookupID(ctx, addr, ts)
}

func (c *FullNodeStruct) StateChangedActors(ctx context.Context, olnstate cid.Cid, newstate cid.Cid) (map[string]types.Actor, error) {
	return c.Internal.StateChangedActors(ctx, olnstate, newstate)
}

func (c *FullNodeStruct) StateGetReceipt(ctx context.Context, msg cid.Cid, ts *types.TipSet) (*types.MessageReceipt, error) {
	return c.Internal.StateGetReceipt(ctx, msg, ts)
}

func (c *FullNodeStruct) MarketEnsureAvailable(ctx context.Context, addr address.Address, amt types.BigInt) error {
	return c.Internal.MarketEnsureAvailable(ctx, addr, amt)
}

func (c *FullNodeStruct) PaychGet(ctx context.Context, from, to address.Address, ensureFunds types.BigInt) (*ChannelInfo, error) {
	return c.Internal.PaychGet(ctx, from, to, ensureFunds)
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

func (c *FullNodeStruct) PaychVoucherAdd(ctx context.Context, addr address.Address, sv *types.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	return c.Internal.PaychVoucherAdd(ctx, addr, sv, proof, minDelta)
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

func (c *FullNodeStruct) PaychAllocateLane(ctx context.Context, ch address.Address) (uint64, error) {
	return c.Internal.PaychAllocateLane(ctx, ch)
}

func (c *FullNodeStruct) PaychNewPayment(ctx context.Context, from, to address.Address, vouchers []VoucherSpec) (*PaymentInfo, error) {
	return c.Internal.PaychNewPayment(ctx, from, to, vouchers)
}

func (c *FullNodeStruct) PaychVoucherSubmit(ctx context.Context, ch address.Address, sv *types.SignedVoucher) (cid.Cid, error) {
	return c.Internal.PaychVoucherSubmit(ctx, ch, sv)
}

func (c *StorageMinerStruct) ActorAddress(ctx context.Context) (address.Address, error) {
	return c.Internal.ActorAddress(ctx)
}

func (c *StorageMinerStruct) StoreGarbageData(ctx context.Context) error {
	return c.Internal.StoreGarbageData(ctx)
}

// Get the status of a given sector by ID
func (c *StorageMinerStruct) SectorsStatus(ctx context.Context, sid uint64) (SectorInfo, error) {
	return c.Internal.SectorsStatus(ctx, sid)
}

// List all staged sectors
func (c *StorageMinerStruct) SectorsList(ctx context.Context) ([]uint64, error) {
	return c.Internal.SectorsList(ctx)
}

func (c *StorageMinerStruct) SectorsRefs(ctx context.Context) (map[string][]SealedRef, error) {
	return c.Internal.SectorsRefs(ctx)
}

func (c *StorageMinerStruct) WorkerStats(ctx context.Context) (WorkerStats, error) {
	return c.Internal.WorkerStats(ctx)
}

var _ Common = &CommonStruct{}
var _ FullNode = &FullNodeStruct{}
var _ StorageMiner = &StorageMinerStruct{}
