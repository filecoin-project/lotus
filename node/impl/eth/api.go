package eth

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var (
	ErrChainIndexerDisabled = xerrors.New("chain indexer is disabled; please enable the ChainIndexer to use the ETH RPC API")
	ErrModuleDisabled       = xerrors.New("module disabled, enable with Fevm.EnableEthRPC / LOTUS_FEVM_ENABLEETHRPC")
)

var log = logging.Logger("node/eth")

// EthFilecoin -------------------------------------------------------------------------------------

type EthFilecoinAPI interface {
	EthAddressToFilecoinAddress(ctx context.Context, ethAddress ethtypes.EthAddress) (address.Address, error)
	FilecoinAddressToEthAddress(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthAddress, error)
}

// EthBasic ----------------------------------------------------------------------------------------

type EthBasicAPI interface {
	Web3ClientVersion(ctx context.Context) (string, error)
	EthChainId(ctx context.Context) (ethtypes.EthUint64, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error)
	EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error)
	EthAccounts(ctx context.Context) ([]ethtypes.EthAddress, error)
}

// EthSend -----------------------------------------------------------------------------------------

type EthSendAPI interface {
	EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error)
	EthSendRawTransactionUntrusted(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error)
}

// EthTransaction ----------------------------------------------------------------------------------

type EthTransactionAPI interface {
	EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error)

	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum string) (ethtypes.EthUint64, error)
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error)
	EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error)

	EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error)
	EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum string, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error)

	EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error)
	EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error)
	EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error)

	EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*ethtypes.EthTxReceipt, error)
	EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTxReceipt, error)
	EthGetBlockReceipts(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) ([]*ethtypes.EthTxReceipt, error)
	EthGetBlockReceiptsLimited(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, limit abi.ChainEpoch) ([]*ethtypes.EthTxReceipt, error)
}

// EthLookup ---------------------------------------------------------------------------------------

type EthLookupAPI interface {
	EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetStorageAt(ctx context.Context, ethAddr ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error)
}

// EthTrace ----------------------------------------------------------------------------------------

type EthTraceAPI interface {
	EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error)
	EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error)
	EthTraceTransaction(ctx context.Context, txHash string) ([]*ethtypes.EthTraceTransaction, error)
	EthTraceFilter(ctx context.Context, filter ethtypes.EthTraceFilterCriteria) ([]*ethtypes.EthTraceFilterResult, error)
}

// EthGas ------------------------------------------------------------------------------------------

type EthGasAPI interface {
	EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error)
	EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error)
	EthEstimateGas(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthUint64, error)
	EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
}

// EthEvents ---------------------------------------------------------------------------------------

const (
	EthSubscribeEventTypeHeads               = "newHeads"
	EthSubscribeEventTypeLogs                = "logs"
	EthSubscribeEventTypePendingTransactions = "newPendingTransactions"
)

type EthEventsAPI interface {
	EthGetLogs(ctx context.Context, filter *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error)
	EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error)
	EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error)
	EthNewFilter(ctx context.Context, filter *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error)
	EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error)
	EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error)
	EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error)
	EthSubscribe(ctx context.Context, params jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error)
	EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error)
}

// EthEventsInternal extends the EthEvents interface with additional methods that are not exposed
// on the JSON-RPC API.
type EthEventsInternal interface {
	EthEventsAPI

	// GetEthLogsForBlockAndTransaction returns the logs for a block and transaction, it is intended
	// for internal use rather than being exposed via the JSON-RPC API.
	GetEthLogsForBlockAndTransaction(ctx context.Context, blockHash *ethtypes.EthHash, txHash ethtypes.EthHash) ([]ethtypes.EthLog, error)
	// GC runs a garbage collection loop, deleting filters that have not been used within the ttl
	// window, it is intended for internal use rather than being exposed via the JSON-RPC API.
	GC(ctx context.Context, ttl time.Duration)
}

// Complete APIs -----------------------------------------------------------------------------------

type EthModuleAPI interface {
	EthFilecoinAPI
	EthBasicAPI
	EthSendAPI
	EthTransactionAPI
	EthLookupAPI
	EthTraceAPI
	EthGasAPI
	EthEventsAPI
}

// Required Dependencies ---------------------------------------------------------------------------

// TipSetResolver is responsible for translating Ethereum API input to tipsets. It may use static
// rules or F3 to resolve "latest" and "safe" tags as appropriate.
//
// In most cases, errors from TipSetResolver should be returned as-is if they are within the
// top-level of a JSONRPC method so ErrNullRound is properly wrapped when encountered.
type TipSetResolver interface {
	GetTipSetByHash(ctx context.Context, blkParam ethtypes.EthHash) (*types.TipSet, error)
	GetTipsetByBlockNumber(ctx context.Context, blkParam string, strict bool) (*types.TipSet, error)
	GetTipsetByBlockNumberOrHash(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) (*types.TipSet, error)
}

// SyncAPI is a minimal version of full.SyncAPI
type SyncAPI interface {
	SyncState(ctx context.Context) (*api.SyncState, error)
}

// ChainStore is a minimal version of store.ChainStore just for tipsets
type ChainStore interface {
	// TipSets
	GetHeaviestTipSet() *types.TipSet
	GetTipsetByHeight(ctx context.Context, h abi.ChainEpoch, ts *types.TipSet, prev bool) (*types.TipSet, error)
	GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	GetTipSetByCid(ctx context.Context, c cid.Cid) (*types.TipSet, error)
	LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)

	// Messages
	GetSignedMessage(ctx context.Context, c cid.Cid) (*types.SignedMessage, error)
	GetMessage(ctx context.Context, c cid.Cid) (*types.Message, error)
	BlockMsgsForTipset(ctx context.Context, ts *types.TipSet) ([]store.BlockMessages, error)
	MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error)
	ReadReceipts(ctx context.Context, root cid.Cid) ([]types.MessageReceipt, error)

	// Misc
	ActorStore(ctx context.Context) adt.Store
}

// StateAPI is a minimal version of full.StateAPI
type StateAPI interface {
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
}

// StateManager is a minimal version of stmgr.StateManager
type StateManager interface {
	GetNetworkVersion(ctx context.Context, height abi.ChainEpoch) network.Version

	TipSetState(ctx context.Context, ts *types.TipSet) (cid.Cid, cid.Cid, error)
	ParentState(ts *types.TipSet) (*state.StateTree, error)
	StateTree(st cid.Cid) (*state.StateTree, error)

	LookupIDAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error)
	LoadActor(ctx context.Context, addr address.Address, ts *types.TipSet) (*types.Actor, error)
	LoadActorRaw(ctx context.Context, addr address.Address, st cid.Cid) (*types.Actor, error)
	ResolveToDeterministicAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error)

	ExecutionTrace(ctx context.Context, ts *types.TipSet) (cid.Cid, []*api.InvocResult, error)
	Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error)
	CallWithGas(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet, applyTsMessages bool) (*api.InvocResult, error)
	ApplyOnStateWithGas(ctx context.Context, stateCid cid.Cid, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error)

	HasExpensiveForkBetween(parent, height abi.ChainEpoch) bool
}

// MpoolAPI is a minimal version of full.MpoolAPI
type MpoolAPI interface {
	MpoolPending(ctx context.Context, tsk types.TipSetKey) ([]*types.SignedMessage, error)
	MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error)
	MpoolPushUntrusted(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error)
	MpoolPush(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error)
}

// MessagePool is a minimal version of messagepool.MessagePool
type MessagePool interface {
	PendingFor(ctx context.Context, a address.Address) ([]*types.SignedMessage, *types.TipSet)
	GetConfig() *types.MpoolConfig
}

// GasAPI is a minimal version of full.GasAPI
type GasAPI interface {
	GasEstimateGasPremium(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, ts types.TipSetKey) (types.BigInt, error)
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, ts types.TipSetKey) (*types.Message, error)
}
