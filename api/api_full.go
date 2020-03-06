package api

import (
	"context"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	"github.com/libp2p/go-libp2p-core/peer"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

// FullNode API is a low-level interface to the Filecoin network full node
type FullNode interface {
	Common

	// TODO: TipSetKeys

	// chain

	// ChainNotify returns channel with chain head updates
	// First message is guaranteed to be of len == 1, and type == 'current'
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainHead(context.Context) (*types.TipSet, error)
	ChainGetRandomness(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
	ChainGetBlock(context.Context, cid.Cid) (*types.BlockHeader, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*BlockMessages, error)
	ChainGetParentReceipts(context.Context, cid.Cid) ([]*types.MessageReceipt, error)
	ChainGetParentMessages(context.Context, cid.Cid) ([]Message, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
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

	MinerCreateBlock(context.Context, address.Address, types.TipSetKey, *types.Ticket, *types.EPostProof, []*types.SignedMessage, abi.ChainEpoch, uint64) (*types.BlockMsg, error)

	// // UX ?

	// wallet

	WalletNew(context.Context, crypto.SigType) (address.Address, error)
	WalletHas(context.Context, address.Address) (bool, error)
	WalletList(context.Context) ([]address.Address, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletSign(context.Context, address.Address, []byte) (*crypto.Signature, error)
	WalletSignMessage(context.Context, address.Address, *types.Message) (*types.SignedMessage, error)
	WalletDefaultAddress(context.Context) (address.Address, error)
	WalletSetDefault(context.Context, address.Address) error
	WalletExport(context.Context, address.Address) (*types.KeyInfo, error)
	WalletImport(context.Context, *types.KeyInfo) (address.Address, error)

	// Other

	// ClientImport imports file under the specified path into filestore
	ClientImport(ctx context.Context, path string) (cid.Cid, error)
	ClientStartDeal(ctx context.Context, params *StartDealParams) (*cid.Cid, error)
	ClientGetDealInfo(context.Context, cid.Cid) (*DealInfo, error)
	ClientListDeals(ctx context.Context) ([]DealInfo, error)
	ClientHasLocal(ctx context.Context, root cid.Cid) (bool, error)
	ClientFindData(ctx context.Context, root cid.Cid) ([]QueryOffer, error)
	ClientRetrieve(ctx context.Context, order RetrievalOrder, path string) error
	ClientQueryAsk(ctx context.Context, p peer.ID, miner address.Address) (*storagemarket.SignedStorageAsk, error)

	// ClientUnimport removes references to the specified file from filestore
	//ClientUnimport(path string)

	// ClientListImports lists imported files and their root CIDs
	ClientListImports(ctx context.Context) ([]Import, error)

	//ClientListAsks() []Ask

	// if tipset is nil, we'll use heaviest
	StateCall(context.Context, *types.Message, types.TipSetKey) (*MethodCall, error)
	StateReplay(context.Context, types.TipSetKey, cid.Cid) (*ReplayResults, error)
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	StateReadState(ctx context.Context, act *types.Actor, tsk types.TipSetKey) (*ActorState, error)
	StateListMessages(ctx context.Context, match *types.Message, tsk types.TipSetKey, toht abi.ChainEpoch) ([]cid.Cid, error)

	StateMinerSectors(context.Context, address.Address, types.TipSetKey) ([]*ChainSectorInfo, error)
	StateMinerProvingSet(context.Context, address.Address, types.TipSetKey) ([]*ChainSectorInfo, error)
	StateMinerPower(context.Context, address.Address, types.TipSetKey) (MinerPower, error)
	StateMinerWorker(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateMinerPeerID(ctx context.Context, m address.Address, tsk types.TipSetKey) (peer.ID, error)
	StateMinerPostState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*miner.PoStState, error)
	StateMinerSectorSize(context.Context, address.Address, types.TipSetKey) (abi.SectorSize, error)
	StateMinerFaults(context.Context, address.Address, types.TipSetKey) ([]abi.SectorNumber, error)
	StatePledgeCollateral(context.Context, types.TipSetKey) (types.BigInt, error)
	StateWaitMsg(context.Context, cid.Cid) (*MsgWait, error)
	StateListMiners(context.Context, types.TipSetKey) ([]address.Address, error)
	StateListActors(context.Context, types.TipSetKey) ([]address.Address, error)
	StateMarketBalance(context.Context, address.Address, types.TipSetKey) (MarketBalance, error)
	StateMarketParticipants(context.Context, types.TipSetKey) (map[string]MarketBalance, error)
	StateMarketDeals(context.Context, types.TipSetKey) (map[string]MarketDeal, error)
	StateMarketStorageDeal(context.Context, abi.DealID, types.TipSetKey) (*MarketDeal, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateChangedActors(context.Context, cid.Cid, cid.Cid) (map[string]types.Actor, error)
	StateGetReceipt(context.Context, cid.Cid, types.TipSetKey) (*types.MessageReceipt, error)
	StateMinerSectorCount(context.Context, address.Address, types.TipSetKey) (MinerSectors, error)
	StateListRewards(context.Context, address.Address, types.TipSetKey) ([]reward.Reward, error)
	StateCompute(context.Context, abi.ChainEpoch, []*types.Message, types.TipSetKey) (cid.Cid, error)

	MsigGetAvailableBalance(context.Context, address.Address, types.TipSetKey) (types.BigInt, error)

	MarketEnsureAvailable(context.Context, address.Address, address.Address, types.BigInt) error
	// MarketFreeBalance

	PaychGet(ctx context.Context, from, to address.Address, ensureFunds types.BigInt) (*ChannelInfo, error)
	PaychList(context.Context) ([]address.Address, error)
	PaychStatus(context.Context, address.Address) (*PaychStatus, error)
	PaychClose(context.Context, address.Address) (cid.Cid, error)
	PaychAllocateLane(ctx context.Context, ch address.Address) (uint64, error)
	PaychNewPayment(ctx context.Context, from, to address.Address, vouchers []VoucherSpec) (*PaymentInfo, error)
	PaychVoucherCheckValid(context.Context, address.Address, *paych.SignedVoucher) error
	PaychVoucherCheckSpendable(context.Context, address.Address, *paych.SignedVoucher, []byte, []byte) (bool, error)
	PaychVoucherCreate(context.Context, address.Address, types.BigInt, uint64) (*paych.SignedVoucher, error)
	PaychVoucherAdd(context.Context, address.Address, *paych.SignedVoucher, []byte, types.BigInt) (types.BigInt, error)
	PaychVoucherList(context.Context, address.Address) ([]*paych.SignedVoucher, error)
	PaychVoucherSubmit(context.Context, address.Address, *paych.SignedVoucher) (cid.Cid, error)
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
	State       storagemarket.StorageDealStatus
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
	Info miner.SectorOnChainInfo
	ID   abi.SectorNumber
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
	Vouchers       []*paych.SignedVoucher
}

type VoucherSpec struct {
	Amount      types.BigInt
	TimeLockMin abi.ChainEpoch
	TimeLockMax abi.ChainEpoch
	MinSettle   abi.ChainEpoch

	Extra *paych.ModVerifyParams
}

type MinerPower struct {
	MinerPower types.BigInt
	TotalPower types.BigInt
}

type QueryOffer struct {
	Err string

	Root cid.Cid

	Size                    uint64
	MinPrice                types.BigInt
	PaymentInterval         uint64
	PaymentIntervalIncrease uint64
	Miner                   address.Address
	MinerPeerID             peer.ID
}

func (o *QueryOffer) Order(client address.Address) RetrievalOrder {
	return RetrievalOrder{
		Root:                    o.Root,
		Size:                    o.Size,
		Total:                   o.MinPrice,
		PaymentInterval:         o.PaymentInterval,
		PaymentIntervalIncrease: o.PaymentIntervalIncrease,
		Client:                  client,

		Miner:       o.Miner,
		MinerPeerID: o.MinerPeerID,
	}
}

type MarketBalance struct {
	Escrow big.Int
	Locked big.Int
}

type MarketDeal struct {
	Proposal market.DealProposal
	State    market.DealState
}

type RetrievalOrder struct {
	// TODO: make this less unixfs specific
	Root cid.Cid
	Size uint64
	// TODO: support offset
	Total                   types.BigInt
	PaymentInterval         uint64
	PaymentIntervalIncrease uint64
	Client                  address.Address
	Miner                   address.Address
	MinerPeerID             peer.ID
}

type ReplayResults struct {
	Msg     *types.Message
	Receipt *types.MessageReceipt
	Error   string
}

type MethodCall struct {
	types.MessageReceipt
	Error string
}

type StartDealParams struct {
	Data           *storagemarket.DataRef
	Wallet         address.Address
	Miner          address.Address
	EpochPrice     types.BigInt
	BlocksDuration uint64
}

type ActiveSync struct {
	Base   *types.TipSet
	Target *types.TipSet

	Stage  SyncStateStage
	Height abi.ChainEpoch

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

func ProofTypeFromSectorSize(ssize abi.SectorSize) (abi.RegisteredProof, abi.RegisteredProof, error) {
	switch ssize {
	case 2 << 10:
		return abi.RegisteredProof_StackedDRG2KiBPoSt, abi.RegisteredProof_StackedDRG2KiBSeal, nil
	case 8 << 20:
		return abi.RegisteredProof_StackedDRG8MiBPoSt, abi.RegisteredProof_StackedDRG8MiBSeal, nil
	case 512 << 20:
		return abi.RegisteredProof_StackedDRG512MiBPoSt, abi.RegisteredProof_StackedDRG512MiBSeal, nil
	case 32 << 30:
		return abi.RegisteredProof_StackedDRG32GiBPoSt, abi.RegisteredProof_StackedDRG32GiBSeal, nil
	default:
		return 0, 0, xerrors.Errorf("unsupported sector size for miner: %v", ssize)
	}
}
