package full

import (
	"context"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/impl/eth"
)

type EthModuleAPI interface {
	EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error)
	EthAccounts(ctx context.Context) ([]ethtypes.EthAddress, error)
	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum ethtypes.EthUint64) (ethtypes.EthUint64, error)
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error)
	EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error)
	EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error)
	EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error)
	EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error)
	EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error)
	EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error)
	EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error)
	EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*api.EthTxReceipt, error)
	EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error)
	EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error)
	EthChainId(ctx context.Context) (ethtypes.EthUint64, error)
	EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error)
	EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error)
	EthEstimateGas(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthUint64, error)
	EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error)
	EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error)
	Web3ClientVersion(ctx context.Context) (string, error)
	EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error)
	EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error)
	EthTraceTransaction(ctx context.Context, txHash string) ([]*ethtypes.EthTraceTransaction, error)
	EthTraceFilter(ctx context.Context, filter ethtypes.EthTraceFilterCriteria) ([]*ethtypes.EthTraceFilterResult, error)
	EthGetBlockReceiptsLimited(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, limit abi.ChainEpoch) ([]*api.EthTxReceipt, error)
	EthGetBlockReceipts(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) ([]*api.EthTxReceipt, error)
}

type EthEventAPI interface {
	EthGetLogs(ctx context.Context, filter *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error)
	EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error)
	EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error)
	EthNewFilter(ctx context.Context, filter *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error)
	EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error)
	EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error)
	EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error)
	EthSubscribe(ctx context.Context, params jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error)
	EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error)
}

var (
	_ EthModuleAPI = *new(api.FullNode)
	_ EthEventAPI  = *new(api.FullNode)
	_ EthModuleAPI = *new(FullEthAPI)
	_ EthEventAPI  = *new(FullEthAPI)
	_ EthModuleAPI = *new(api.Gateway)
	_ EthEventAPI  = *new(api.Gateway)
)

type FullEthAPI struct {
	fx.In

	eth.EthFilecoinAPI
	eth.EthBasicAPI
	eth.EthTransactionAPI
	eth.EthLookupAPI
	eth.EthTraceAPI
	eth.EthGasAPI
	eth.EthEventsAPI
	eth.EthSendAPI
}
