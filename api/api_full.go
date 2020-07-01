package api

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// FullNode API is a low-level interface to the Filecoin network full node
type FullNode interface {
	Common

	// TODO: TipSetKeys

	// MethodGroup: Chain
	// The Chain method group contains methods for interacting with the
	// blockchain, but that do not require any form of state computation

	// ChainNotify returns channel with chain head updates
	// First message is guaranteed to be of len == 1, and type == 'current'
	ChainNotify(context.Context) (<-chan []*HeadChange, error)

	// ChainHead returns the current head of the chain
	ChainHead(context.Context) (*types.TipSet, error)

	// ChainGetRandomness is used to sample the chain for randomness
	ChainGetRandomness(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)

	// ChainGetBlock returns the block specified by the given CID
	ChainGetBlock(context.Context, cid.Cid) (*types.BlockHeader, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)

	// ChainGetBlockMessages returns messages stored in the specified block
	ChainGetBlockMessages(ctx context.Context, blockCid cid.Cid) (*BlockMessages, error)

	// ChainGetParentReceipts returns receipts for messages in parent tipset of
	// the specified block
	ChainGetParentReceipts(ctx context.Context, blockCid cid.Cid) ([]*types.MessageReceipt, error)

	// ChainGetParentReceipts returns messages stored in parent tipset of the
	// specified block
	ChainGetParentMessages(ctx context.Context, blockCid cid.Cid) ([]Message, error)

	// ChainGetTipSetByHeight looks back for a tipset at the specified epoch.
	// If there are no blocks at the specified epoch, a tipset at higher epoch
	// will be returned
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)

	// ChainReadObj reads ipld nodes referenced by the specified CID from chain
	// blockstore and returns raw bytes
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)

	// ChainHasObj checks if a given CID exists in the chain blockstore
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainStatObj(context.Context, cid.Cid, cid.Cid) (ObjStat, error)

	// ChainSetHead forcefully sets current chain head. Use with caution
	ChainSetHead(context.Context, types.TipSetKey) error

	// ChainGetGenesis returns the genesis tipset
	ChainGetGenesis(context.Context) (*types.TipSet, error)

	// ChainTipSetWeight computes weight for the specified tipset
	ChainTipSetWeight(context.Context, types.TipSetKey) (types.BigInt, error)
	ChainGetNode(ctx context.Context, p string) (*IpldObject, error)

	// ChainGetMessage reads a message referenced by the specified CID from the
	// chain blockstore
	ChainGetMessage(context.Context, cid.Cid) (*types.Message, error)

	// ChainGetPath returns a set of revert/apply operations needed to get from
	// one tipset to another, for example:
	//```
	//        to
	//         ^
	// from   tAA
	//   ^     ^
	// tBA    tAB
	//  ^---*--^
	//      ^
	//     tRR
	//```
	// Would return `[revert(tBA), apply(tAB), apply(tAA)]`
	ChainGetPath(ctx context.Context, from types.TipSetKey, to types.TipSetKey) ([]*HeadChange, error)

	// ChainExport returns a stream of bytes with CAR dump of chain data
	ChainExport(context.Context, types.TipSetKey) (<-chan []byte, error)

	// MethodGroup: Sync
	// The Sync method group contains methods for interacting with and
	// observing the lotus sync service

	// SyncState returns the current status of the lotus sync system
	SyncState(context.Context) (*SyncState, error)

	// SyncSubmitBlock can be used to submit a newly created block to the
	// network through this node
	SyncSubmitBlock(ctx context.Context, blk *types.BlockMsg) error

	// SyncIncomingBlocks returns a channel streaming incoming, potentially not
	// yet synced block headers.
	SyncIncomingBlocks(ctx context.Context) (<-chan *types.BlockHeader, error)

	// SyncMarkBad marks a blocks as bad, meaning that it won't ever by synced.
	// Use with extreme caution
	SyncMarkBad(ctx context.Context, bcid cid.Cid) error

	// SyncCheckBad checks if a block was marked as bad, and if it was, returns
	// the reason
	SyncCheckBad(ctx context.Context, bcid cid.Cid) (string, error)

	// MethodGroup: Mpool
	// The Mpool methods are for interacting with the message pool. The message pool
	// manages all incoming and outgoing 'messages' going over the network.

	// MpoolPending returns pending mempool messages
	MpoolPending(context.Context, types.TipSetKey) ([]*types.SignedMessage, error)

	// MpoolPush pushes a signed message to mempool
	MpoolPush(context.Context, *types.SignedMessage) (cid.Cid, error)

	// MpoolPushMessage atomically assigns a nonce, signs, and pushes a message
	// to mempool
	MpoolPushMessage(context.Context, *types.Message) (*types.SignedMessage, error)

	// MpoolGetNonce gets next nonce for the specified sender.
	// Note that this method may not be atomic. Use MpoolPushMessage instead
	MpoolGetNonce(context.Context, address.Address) (uint64, error)
	MpoolSub(context.Context) (<-chan MpoolUpdate, error)

	// MpoolEstimateGasPrice estimates what gas price should be used for a
	// message to have high likelihood of inclusion in `nblocksincl` epochs
	MpoolEstimateGasPrice(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error)

	// MethodGroup: Miner

	MinerGetBaseInfo(context.Context, address.Address, abi.ChainEpoch, types.TipSetKey) (*MiningBaseInfo, error)
	MinerCreateBlock(context.Context, *BlockTemplate) (*types.BlockMsg, error)

	// // UX ?

	// MethodGroup: Wallet

	WalletNew(context.Context, crypto.SigType) (address.Address, error)
	WalletHas(context.Context, address.Address) (bool, error)
	WalletList(context.Context) ([]address.Address, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletSign(context.Context, address.Address, []byte) (*crypto.Signature, error)
	WalletSignMessage(context.Context, address.Address, *types.Message) (*types.SignedMessage, error)
	WalletVerify(context.Context, address.Address, []byte, *crypto.Signature) bool
	WalletDefaultAddress(context.Context) (address.Address, error)
	WalletSetDefault(context.Context, address.Address) error
	WalletExport(context.Context, address.Address) (*types.KeyInfo, error)
	WalletImport(context.Context, *types.KeyInfo) (address.Address, error)
	WalletDelete(context.Context, address.Address) error

	// Other

	// MethodGroup: Client
	// The Client methods all have to do with interacting with the storage and
	// retrieval markets as a client

	// ClientImport imports file under the specified path into filestore
	ClientImport(ctx context.Context, ref FileRef) (cid.Cid, error)
	// ClientStartDeal proposes a deal with a miner
	ClientStartDeal(ctx context.Context, params *StartDealParams) (*cid.Cid, error)
	// ClientGetDeal info returns the latest information about a given deal
	ClientGetDealInfo(context.Context, cid.Cid) (*DealInfo, error)
	ClientListDeals(ctx context.Context) ([]DealInfo, error)
	ClientHasLocal(ctx context.Context, root cid.Cid) (bool, error)
	ClientFindData(ctx context.Context, root cid.Cid) ([]QueryOffer, error)
	ClientMinerQueryOffer(ctx context.Context, root cid.Cid, miner address.Address) (QueryOffer, error)
	ClientRetrieve(ctx context.Context, order RetrievalOrder, ref *FileRef) error
	ClientQueryAsk(ctx context.Context, p peer.ID, miner address.Address) (*storagemarket.SignedStorageAsk, error)
	ClientCalcCommP(ctx context.Context, inpath string, miner address.Address) (*CommPRet, error)
	ClientGenCar(ctx context.Context, ref FileRef, outpath string) error

	// ClientUnimport removes references to the specified file from filestore
	//ClientUnimport(path string)

	// ClientListImports lists imported files and their root CIDs
	ClientListImports(ctx context.Context) ([]Import, error)

	//ClientListAsks() []Ask

	// MethodGroup: State
	// The State methods are used to query, inspect, and interact with chain state

	// if tipset is nil, we'll use heaviest
	StateCall(context.Context, *types.Message, types.TipSetKey) (*InvocResult, error)
	StateReplay(context.Context, types.TipSetKey, cid.Cid) (*InvocResult, error)
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*ActorState, error)
	StateListMessages(ctx context.Context, match *types.Message, tsk types.TipSetKey, toht abi.ChainEpoch) ([]cid.Cid, error)

	StateNetworkName(context.Context) (dtypes.NetworkName, error)
	StateMinerSectors(context.Context, address.Address, *abi.BitField, bool, types.TipSetKey) ([]*ChainSectorInfo, error)
	StateMinerProvingSet(context.Context, address.Address, types.TipSetKey) ([]*ChainSectorInfo, error)
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*miner.DeadlineInfo, error)
	StateMinerPower(context.Context, address.Address, types.TipSetKey) (*MinerPower, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (MinerInfo, error)
	StateMinerDeadlines(context.Context, address.Address, types.TipSetKey) (*miner.Deadlines, error)
	StateMinerFaults(context.Context, address.Address, types.TipSetKey) (*abi.BitField, error)
	// Returns all non-expired Faults that occur within lookback epochs of the given tipset
	StateAllMinerFaults(ctx context.Context, lookback abi.ChainEpoch, ts types.TipSetKey) ([]*Fault, error)
	StateMinerRecoveries(context.Context, address.Address, types.TipSetKey) (*abi.BitField, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (types.BigInt, error)
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (types.BigInt, error)
	StateSectorPreCommitInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error)
	StateSectorGetInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorOnChainInfo, error)
	StatePledgeCollateral(context.Context, types.TipSetKey) (types.BigInt, error)
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64) (*MsgLookup, error)
	StateSearchMsg(context.Context, cid.Cid) (*MsgLookup, error)
	StateListMiners(context.Context, types.TipSetKey) ([]address.Address, error)
	StateListActors(context.Context, types.TipSetKey) ([]address.Address, error)
	StateMarketBalance(context.Context, address.Address, types.TipSetKey) (MarketBalance, error)
	StateMarketParticipants(context.Context, types.TipSetKey) (map[string]MarketBalance, error)
	StateMarketDeals(context.Context, types.TipSetKey) (map[string]MarketDeal, error)
	StateMarketStorageDeal(context.Context, abi.DealID, types.TipSetKey) (*MarketDeal, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateChangedActors(context.Context, cid.Cid, cid.Cid) (map[string]types.Actor, error)
	StateGetReceipt(context.Context, cid.Cid, types.TipSetKey) (*types.MessageReceipt, error)
	StateMinerSectorCount(context.Context, address.Address, types.TipSetKey) (MinerSectors, error)
	StateCompute(context.Context, abi.ChainEpoch, []*types.Message, types.TipSetKey) (*ComputeStateOutput, error)

	// MethodGroup: Msig
	// The Msig methods are used to interact with multisig wallets on the
	// filecoin network

	MsigGetAvailableBalance(context.Context, address.Address, types.TipSetKey) (types.BigInt, error)
	MsigCreate(context.Context, int64, []address.Address, types.BigInt, address.Address, types.BigInt) (cid.Cid, error)
	MsigPropose(context.Context, address.Address, address.Address, types.BigInt, address.Address, uint64, []byte) (cid.Cid, error)
	MsigApprove(context.Context, address.Address, uint64, address.Address, address.Address, types.BigInt, address.Address, uint64, []byte) (cid.Cid, error)
	MsigCancel(context.Context, address.Address, uint64, address.Address, address.Address, types.BigInt, address.Address, uint64, []byte) (cid.Cid, error)

	MarketEnsureAvailable(context.Context, address.Address, address.Address, types.BigInt) (cid.Cid, error)
	// MarketFreeBalance

	// MethodGroup: Paych
	// The Paych methods are for interacting with and managing payment channels

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

type FileRef struct {
	Path  string
	IsCAR bool
}

type MinerSectors struct {
	Sset uint64
	Pset uint64
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
	Message     string // more information about deal state, particularly errors
	Provider    address.Address

	PieceCID cid.Cid
	Size     uint64

	PricePerEpoch types.BigInt
	Duration      uint64

	DealID abi.DealID
}

type MsgLookup struct {
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
	MinerPower power.Claim
	TotalPower power.Claim
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

type InvocResult struct {
	Msg            *types.Message
	MsgRct         *types.MessageReceipt
	ExecutionTrace types.ExecutionTrace
	Error          string
	Duration       time.Duration
}

type MethodCall struct {
	types.MessageReceipt
	Error string
}

type StartDealParams struct {
	Data              *storagemarket.DataRef
	Wallet            address.Address
	Miner             address.Address
	EpochPrice        types.BigInt
	MinBlocksDuration uint64
	DealStartEpoch    abi.ChainEpoch
}

type IpldObject struct {
	Cid cid.Cid
	Obj interface{}
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

type ComputeStateOutput struct {
	Root  cid.Cid
	Trace []*InvocResult
}

type MiningBaseInfo struct {
	MinerPower      types.BigInt
	NetworkPower    types.BigInt
	Sectors         []abi.SectorInfo
	WorkerKey       address.Address
	SectorSize      abi.SectorSize
	PrevBeaconEntry types.BeaconEntry
	BeaconEntries   []types.BeaconEntry
}

type BlockTemplate struct {
	Miner            address.Address
	Parents          types.TipSetKey
	Ticket           *types.Ticket
	Eproof           *types.ElectionProof
	BeaconValues     []types.BeaconEntry
	Messages         []*types.SignedMessage
	Epoch            abi.ChainEpoch
	Timestamp        uint64
	WinningPoStProof []abi.PoStProof
}

type CommPRet struct {
	Root cid.Cid
	Size abi.UnpaddedPieceSize
}
type HeadChange struct {
	Type string
	Val  *types.TipSet
}

type MsigProposeResponse int

const (
	MsigApprove MsigProposeResponse = iota
	MsigCancel
)

type Fault struct {
	Miner address.Address
	Epoch abi.ChainEpoch
}
