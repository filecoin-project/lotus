package full

import (
	"context"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/impl/eth"
)

type ethModuleAPI interface {
	// EthFilecoinAPI
	EthAddressToFilecoinAddress(ctx context.Context, ethAddress ethtypes.EthAddress) (address.Address, error)
	FilecoinAddressToEthAddress(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthAddress, error)

	// EthBasicAPI
	Web3ClientVersion(ctx context.Context) (string, error)
	EthChainId(ctx context.Context) (ethtypes.EthUint64, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error)
	EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error)
	EthAccounts(ctx context.Context) ([]ethtypes.EthAddress, error)

	// EthSendAPI
	EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error)

	// EthTransactionAPI
	EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error)

	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum string) (ethtypes.EthUint64, error)
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error)
	EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error)

	EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum string, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error)

	EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error)
	EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error)
	EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error)

	EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error)
	EthGetBlockReceipts(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) ([]*api.EthTxReceipt, error)

	// EthLookupAPI
	EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetStorageAt(ctx context.Context, ethAddr ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error)

	// EthTraceAPI
	EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error)
	EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error)
	EthTraceTransaction(ctx context.Context, txHash string) ([]*ethtypes.EthTraceTransaction, error)
	EthTraceFilter(ctx context.Context, filter ethtypes.EthTraceFilterCriteria) ([]*ethtypes.EthTraceFilterResult, error)

	// EthGasAPI
	EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error)
	EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error)
	EthEstimateGas(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthUint64, error)
	EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)

	// EthEventsAPI
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

type fullEthModuleAPI interface {
	ethModuleAPI

	// EthSendAPI
	EthSendRawTransactionUntrusted(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error)

	// EthTransactionAPI
	EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error)
	EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*api.EthTxReceipt, error)
	EthGetBlockReceiptsLimited(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, limit abi.ChainEpoch) ([]*api.EthTxReceipt, error)
}

var (
	_ fullEthModuleAPI = *new(api.FullNode)
	_ fullEthModuleAPI = *new(FullEthAPIV1)
	_ ethModuleAPI     = *new(api.Gateway)
)

// The Eth*V{1,2} interfaces are distinct interface for DI purposes. By making them separate, we can
// construct separate modules for v1 and v2 APIs and provide different TipSetResolvers for them.

type EthTipSetResolverV1 interface{ eth.TipSetResolver }
type EthFilecoinAPIV1 interface{ eth.EthFilecoinAPI }
type EthTransactionAPIV1 interface{ eth.EthTransactionAPI }
type EthLookupAPIV1 interface{ eth.EthLookupAPI }
type EthTraceAPIV1 interface{ eth.EthTraceAPI }
type EthGasAPIV1 interface{ eth.EthGasAPI }

type EthTipSetResolverV2 interface{ eth.TipSetResolver }
type EthFilecoinAPIV2 interface{ eth.EthFilecoinAPI }
type EthTransactionAPIV2 interface{ eth.EthTransactionAPI }
type EthLookupAPIV2 interface{ eth.EthLookupAPI }
type EthTraceAPIV2 interface{ eth.EthTraceAPI }
type EthGasAPIV2 interface{ eth.EthGasAPI }

type FullEthAPIV1 struct {
	fx.In

	EthFilecoinAPIV1
	eth.EthBasicAPI
	eth.EthSendAPI
	EthTransactionAPIV1
	EthLookupAPIV1
	EthTraceAPIV1
	EthGasAPIV1
	eth.EthEventsAPI
}

type FullEthAPIV2 struct {
	fx.In

	EthFilecoinAPIV2
	eth.EthBasicAPI
	eth.EthSendAPI
	EthTransactionAPIV2
	EthLookupAPIV2
	EthTraceAPIV2
	EthGasAPIV2
	eth.EthEventsAPI
}
