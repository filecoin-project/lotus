package api

import apitypes "github.com/filecoin-project/lotus/api/types"

func CreateEthRPCAliases(as apitypes.Aliaser) {
	// TODO: maybe use reflect to automatically register all the eth aliases
	as.AliasMethod("eth_accounts", "Filecoin.EthAccounts")
	as.AliasMethod("eth_blockNumber", "Filecoin.EthBlockNumber")
	as.AliasMethod("eth_getBlockTransactionCountByNumber", "Filecoin.EthGetBlockTransactionCountByNumber")
	as.AliasMethod("eth_getBlockTransactionCountByHash", "Filecoin.EthGetBlockTransactionCountByHash")

	as.AliasMethod("eth_getBlockByHash", "Filecoin.EthGetBlockByHash")
	as.AliasMethod("eth_getBlockByNumber", "Filecoin.EthGetBlockByNumber")
	as.AliasMethod("eth_getTransactionByHash", "Filecoin.EthGetTransactionByHash")
	as.AliasMethod("eth_getTransactionCount", "Filecoin.EthGetTransactionCount")
	as.AliasMethod("eth_getTransactionReceipt", "Filecoin.EthGetTransactionReceipt")
	as.AliasMethod("eth_getBlockReceipts", "Filecoin.EthGetBlockReceipts")
	as.AliasMethod("eth_getTransactionByBlockHashAndIndex", "Filecoin.EthGetTransactionByBlockHashAndIndex")
	as.AliasMethod("eth_getTransactionByBlockNumberAndIndex", "Filecoin.EthGetTransactionByBlockNumberAndIndex")

	as.AliasMethod("eth_getCode", "Filecoin.EthGetCode")
	as.AliasMethod("eth_getStorageAt", "Filecoin.EthGetStorageAt")
	as.AliasMethod("eth_getBalance", "Filecoin.EthGetBalance")
	as.AliasMethod("eth_chainId", "Filecoin.EthChainId")
	as.AliasMethod("eth_syncing", "Filecoin.EthSyncing")
	as.AliasMethod("eth_feeHistory", "Filecoin.EthFeeHistory")
	as.AliasMethod("eth_protocolVersion", "Filecoin.EthProtocolVersion")
	as.AliasMethod("eth_maxPriorityFeePerGas", "Filecoin.EthMaxPriorityFeePerGas")
	as.AliasMethod("eth_gasPrice", "Filecoin.EthGasPrice")
	as.AliasMethod("eth_sendRawTransaction", "Filecoin.EthSendRawTransaction")
	as.AliasMethod("eth_estimateGas", "Filecoin.EthEstimateGas")
	as.AliasMethod("eth_call", "Filecoin.EthCall")

	as.AliasMethod("eth_getLogs", "Filecoin.EthGetLogs")
	as.AliasMethod("eth_getFilterChanges", "Filecoin.EthGetFilterChanges")
	as.AliasMethod("eth_getFilterLogs", "Filecoin.EthGetFilterLogs")
	as.AliasMethod("eth_newFilter", "Filecoin.EthNewFilter")
	as.AliasMethod("eth_newBlockFilter", "Filecoin.EthNewBlockFilter")
	as.AliasMethod("eth_newPendingTransactionFilter", "Filecoin.EthNewPendingTransactionFilter")
	as.AliasMethod("eth_uninstallFilter", "Filecoin.EthUninstallFilter")
	as.AliasMethod("eth_subscribe", "Filecoin.EthSubscribe")
	as.AliasMethod("eth_unsubscribe", "Filecoin.EthUnsubscribe")

	as.AliasMethod("trace_block", "Filecoin.EthTraceBlock")
	as.AliasMethod("trace_replayBlockTransactions", "Filecoin.EthTraceReplayBlockTransactions")
	as.AliasMethod("trace_transaction", "Filecoin.EthTraceTransaction")
	as.AliasMethod("trace_filter", "Filecoin.EthTraceFilter")

	as.AliasMethod("net_version", "Filecoin.NetVersion")
	as.AliasMethod("net_listening", "Filecoin.NetListening")

	as.AliasMethod("web3_clientVersion", "Filecoin.Web3ClientVersion")
}
