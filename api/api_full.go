package api

import (
	"context"
	"github.com/filecoin-project/lotus/chain/vm"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

// FullNode API is a low-level interface to the Filecoin network full node
type FullNode interface {
	Common

	// chain

	// ChainNotify returns channel with chain head updates
	// First message is guaranteed to be of len == 1, and type == 'current'
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainHead(context.Context) (*types.TipSet, error)
	ChainGetRandomness(context.Context, types.TipSetKey, int64) ([]byte, error)
	ChainGetBlock(context.Context, cid.Cid) (*types.BlockHeader, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*BlockMessages, error)
	ChainGetParentReceipts(context.Context, cid.Cid) ([]*types.MessageReceipt, error)
	ChainGetParentMessages(context.Context, cid.Cid) ([]Message, error)
	ChainGetTipSetByHeight(context.Context, uint64, types.TipSetKey) (*types.TipSet, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainSetHead(context.Context, types.TipSetKey) error
	ChainGetGenesis(context.Context) (*types.TipSet, error)
	ChainTipSetWeight(context.Context, types.TipSetKey) (types.BigInt, error)
	ChainGetNode(ctx context.Context, p string) (interface{}, error)
	ChainGetMessage(context.Context, cid.Cid) (*types.Message, error)
	ChainGetPath(ctx context.Context, from types.TipSetKey, to types.TipSetKey) ([]*store.HeadChange, error)
	ChainExport(context.Context, types.TipSetKey) (<-chan []byte, error)

	// syncer
	SyncState(context.Context) (*SyncState, error)
	SyncSubmitBlock(ctx context.Context, blk *types.BlockMsg) error
	SyncIncomingBlocks(ctx context.Context) (<-chan *types.BlockHeader, error)
	SyncMarkBad(ctx context.Context, bcid cid.Cid) error
	SyncCheckBad(ctx context.Context, bcid cid.Cid) (string, error)

	// messages
	MpoolPending(context.Context, types.TipSetKey) ([]*types.SignedMessage, error)
	MpoolPush(context.Context, *types.SignedMessage) (cid.Cid, error)
	MpoolPushMessage(context.Context, *types.Message) (*types.SignedMessage, error) // get nonce, sign, push
	MpoolGetNonce(context.Context, address.Address) (uint64, error)
	MpoolSub(context.Context) (<-chan MpoolUpdate, error)

	// FullNodeStruct

	// miner

	MinerCreateBlock(context.Context, address.Address, types.TipSetKey, *types.Ticket, *types.EPostProof, []*types.SignedMessage, uint64, uint64) (*types.BlockMsg, error)

	// // UX ?

	// wallet

	WalletNew(context.Context, string) (address.Address, error)
	WalletHas(context.Context, address.Address) (bool, error)
	WalletList(context.Context) ([]address.Address, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletSign(context.Context, address.Address, []byte) (*types.Signature, error)
	WalletSignMessage(context.Context, address.Address, *types.Message) (*types.SignedMessage, error)
	WalletVerify(context.Context, address.Address, []byte, *types.Signature) bool
	WalletDefaultAddress(context.Context) (address.Address, error)
	WalletSetDefault(context.Context, address.Address) error
	WalletExport(context.Context, address.Address) (*types.KeyInfo, error)
	WalletImport(context.Context, *types.KeyInfo) (address.Address, error)

	// Other

	// ClientImport imports file under the specified path into filestore
	ClientImport(ctx context.Context, path string) (cid.Cid, error)
	ClientStartDeal(ctx context.Context, data cid.Cid, addr address.Address, miner address.Address, epochPrice types.BigInt, blocksDuration uint64) (*cid.Cid, error)
	ClientGetDealInfo(context.Context, cid.Cid) (*DealInfo, error)
	ClientListDeals(ctx context.Context) ([]DealInfo, error)
	ClientHasLocal(ctx context.Context, root cid.Cid) (bool, error)
	ClientFindData(ctx context.Context, root cid.Cid) ([]QueryOffer, error)
	ClientRetrieve(ctx context.Context, order RetrievalOrder, path string) error
	ClientQueryAsk(ctx context.Context, p peer.ID, miner address.Address) (*types.SignedStorageAsk, error)

	// ClientUnimport removes references to the specified file from filestore
	//ClientUnimport(path string)

	// ClientListImports lists imported files and their root CIDs
	ClientListImports(ctx context.Context) ([]Import, error)

	//ClientListAsks() []Ask

	// if tipset is nil, we'll use heaviest
	StateCall(context.Context, *types.Message, types.TipSetKey) (*InvocResult, error)
	StateReplay(context.Context, types.TipSetKey, cid.Cid) (*InvocResult, error)
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	StateReadState(ctx context.Context, act *types.Actor, tsk types.TipSetKey) (*ActorState, error)
	StateListMessages(ctx context.Context, match *types.Message, tsk types.TipSetKey, toht uint64) ([]cid.Cid, error)

	StateMinerSectors(context.Context, address.Address, types.TipSetKey) ([]*ChainSectorInfo, error)
	StateMinerProvingSet(context.Context, address.Address, types.TipSetKey) ([]*ChainSectorInfo, error)
	StateMinerPower(context.Context, address.Address, types.TipSetKey) (MinerPower, error)
	StateMinerWorker(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateMinerPeerID(ctx context.Context, m address.Address, tsk types.TipSetKey) (peer.ID, error)
	StateMinerElectionPeriodStart(ctx context.Context, actor address.Address, tsk types.TipSetKey) (uint64, error)
	StateMinerSectorSize(context.Context, address.Address, types.TipSetKey) (uint64, error)
	StateMinerFaults(context.Context, address.Address, types.TipSetKey) ([]uint64, error)
	StatePledgeCollateral(context.Context, types.TipSetKey) (types.BigInt, error)
	StateWaitMsg(context.Context, cid.Cid) (*MsgWait, error)
	StateListMiners(context.Context, types.TipSetKey) ([]address.Address, error)
	StateListActors(context.Context, types.TipSetKey) ([]address.Address, error)
	StateMarketBalance(context.Context, address.Address, types.TipSetKey) (actors.StorageParticipantBalance, error)
	StateMarketParticipants(context.Context, types.TipSetKey) (map[string]actors.StorageParticipantBalance, error)
	StateMarketDeals(context.Context, types.TipSetKey) (map[string]actors.OnChainDeal, error)
	StateMarketStorageDeal(context.Context, uint64, types.TipSetKey) (*actors.OnChainDeal, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateChangedActors(context.Context, cid.Cid, cid.Cid) (map[string]types.Actor, error)
	StateGetReceipt(context.Context, cid.Cid, types.TipSetKey) (*types.MessageReceipt, error)
	StateMinerSectorCount(context.Context, address.Address, types.TipSetKey) (MinerSectors, error)
	StateCompute(context.Context, uint64, []*types.Message, types.TipSetKey) (cid.Cid, error)

	MsigGetAvailableBalance(context.Context, address.Address, types.TipSetKey) (types.BigInt, error)

	MarketEnsureAvailable(context.Context, address.Address, types.BigInt) error
	// MarketFreeBalance

	PaychGet(ctx context.Context, from, to address.Address, ensureFunds types.BigInt) (*ChannelInfo, error)
	PaychList(context.Context) ([]address.Address, error)
	PaychStatus(context.Context, address.Address) (*PaychStatus, error)
	PaychClose(context.Context, address.Address) (cid.Cid, error)
	PaychAllocateLane(ctx context.Context, ch address.Address) (uint64, error)
	PaychNewPayment(ctx context.Context, from, to address.Address, vouchers []VoucherSpec) (*PaymentInfo, error)
	PaychVoucherCheckValid(context.Context, address.Address, *types.SignedVoucher) error
	PaychVoucherCheckSpendable(context.Context, address.Address, *types.SignedVoucher, []byte, []byte) (bool, error)
	PaychVoucherCreate(context.Context, address.Address, types.BigInt, uint64) (*types.SignedVoucher, error)
	PaychVoucherAdd(context.Context, address.Address, *types.SignedVoucher, []byte, types.BigInt) (types.BigInt, error)
	PaychVoucherList(context.Context, address.Address) ([]*types.SignedVoucher, error)
	PaychVoucherSubmit(context.Context, address.Address, *types.SignedVoucher) (cid.Cid, error)
}

type MinerSectors struct {
	Pset uint64
	Sset uint64
}

type Import struct {
	Status   filestore.Status
	Key      cid.Cid
	FilePath string
	Size     uint64
}

type DealInfo struct {
	ProposalCid cid.Cid
	State       DealState
	Provider    address.Address

	PieceRef []byte // cid bytes
	Size     uint64

	PricePerEpoch types.BigInt
	Duration      uint64
}

type MsgWait struct {
	Receipt types.MessageReceipt
	TipSet  *types.TipSet
}

type BlockMessages struct {
	BlsMessages   []*types.Message
	SecpkMessages []*types.SignedMessage

	Cids []cid.Cid
}

type Message struct {
	Cid     cid.Cid
	Message *types.Message
}

type ChainSectorInfo struct {
	SectorID uint64
	CommD    []byte
	CommR    []byte
}

type ActorState struct {
	Balance types.BigInt
	State   interface{}
}

type PCHDir int

const (
	PCHUndef PCHDir = iota
	PCHInbound
	PCHOutbound
)

type PaychStatus struct {
	ControlAddr address.Address
	Direction   PCHDir
}

type ChannelInfo struct {
	Channel        address.Address
	ChannelMessage cid.Cid
}

type PaymentInfo struct {
	Channel        address.Address
	ChannelMessage *cid.Cid
	Vouchers       []*types.SignedVoucher
}

type VoucherSpec struct {
	Amount   types.BigInt
	TimeLock uint64
	MinClose uint64

	Extra *types.ModVerifyParams
}

type MinerPower struct {
	MinerPower types.BigInt
	TotalPower types.BigInt
}

type QueryOffer struct {
	Err string

	Root cid.Cid

	Size     uint64
	MinPrice types.BigInt

	Miner       address.Address
	MinerPeerID peer.ID
}

func (o *QueryOffer) Order(client address.Address) RetrievalOrder {
	return RetrievalOrder{
		Root:  o.Root,
		Size:  o.Size,
		Total: o.MinPrice,

		Client: client,

		Miner:       o.Miner,
		MinerPeerID: o.MinerPeerID,
	}
}

type RetrievalOrder struct {
	// TODO: make this less unixfs specific
	Root cid.Cid
	Size uint64
	// TODO: support offset
	Total types.BigInt

	Client      address.Address
	Miner       address.Address
	MinerPeerID peer.ID
}

type InvocResult struct {
	Msg *types.Message
	MsgRct *types.MessageReceipt
	InternalExecutions []*vm.ExecutionResult
	Error   string
}

type ActiveSync struct {
	Base   *types.TipSet
	Target *types.TipSet

	Stage  SyncStateStage
	Height uint64

	Start   time.Time
	End     time.Time
	Message string
}

type SyncState struct {
	ActiveSyncs []ActiveSync
}

type SyncStateStage int

const (
	StageIdle = SyncStateStage(iota)
	StageHeaders
	StagePersistHeaders
	StageMessages
	StageSyncComplete
	StageSyncErrored
)

type MpoolChange int

const (
	MpoolAdd MpoolChange = iota
	MpoolRemove
)

type MpoolUpdate struct {
	Type    MpoolChange
	Message *types.SignedMessage
}
