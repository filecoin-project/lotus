package v2api

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

//go:generate go run github.com/golang/mock/mockgen -destination=v2mocks/mock_full.go -package=v2mocks . FullNode

// FullNode represents an interface for the v2 full node APIs. This interface
// currently consists of chain-related functionalities and the API is
// experimental and therefore subject to change as we explore the appropriate
// design for the Filecoin v2 APIs.
// The v1 API is stable and is recommended where stable APIs are preferred.
type FullNode interface {
	// MethodGroup: Chain
	// The Chain method group contains methods for interacting with
	// the blockchain.
	//
	// <b>Note: This API is experimental and may change as we explore the
	// appropriate design for the Filecoin v2 APIs.<b/>
	//
	// Please see Filecoin V2 API design documentation for more details:
	//   - https://www.notion.so/filecoindev/Lotus-F3-aware-APIs-1cfdc41950c180ae97fef580e79427d5
	//   - https://www.notion.so/filecoindev/Filecoin-V2-APIs-1d0dc41950c1808b914de5966d501658

	// ChainGetTipSet retrieves a tipset that corresponds to the specified selector
	// criteria. The criteria can be provided in the form of a tipset key, a
	// blockchain height including an optional fallback to previous non-null tipset,
	// or a designated tag such as "latest", "finalized", or "safe".
	//
	// The "Finalized" tag returns the tipset that is considered finalized based on
	// the consensus protocol of the current node, either Filecoin EC Finality or
	// Filecoin Fast Finality (F3). The finalized tipset selection gracefully falls
	// back to EC finality in cases where F3 isn't ready or not running.
	//
	// The "Safe" tag returns the tipset between the "Finalized" tipset and
	// "Latest - build.SafeHeightDistance". This provides a balance between
	// finality confidence and recency. If the tipset at the safe height is null,
	// the first non-nil parent tipset is returned, similar to the behavior of
	// selecting by height with the 'previous' option set to true.
	//
	// In a case where no selector is provided, an error is returned. The selector
	// must be explicitly specified.
	//
	// For more details, refer to the types.TipSetSelector.
	//
	// Example usage:
	//
	//  selector := types.TipSetSelectors.Latest
	//  tipSet, err := node.ChainGetTipSet(context.Background(), selector)
	//  if err != nil {
	//     fmt.Println("Error retrieving tipset:", err)
	//     return
	//  }
	//  fmt.Printf("Latest TipSet: %v\n", tipSet)
	//
	ChainGetTipSet(context.Context, types.TipSetSelector) (*types.TipSet, error) //perm:read

	// MethodGroup: State
	// The State method group contains methods for interacting with the Filecoin
	// blockchain state, including actor information, addresses, and chain data.
	// These methods allow querying the blockchain state at any point in its history
	// using flexible TipSet selection mechanisms.

	// StateGetActor retrieves the actor information for the specified address at the
	// selected tipset.
	//
	// This function returns the on-chain Actor object including:
	//   - Code CID (determines the actor's type)
	//   - State root CID
	//   - Balance in attoFIL
	//   - Nonce (for account actors)
	//
	// The TipSetSelector parameter provides flexible options for selecting the tipset:
	//   - TipSetSelectors.Latest: the most recent tipset with the heaviest weight
	//   - TipSetSelectors.Finalized: the most recent finalized tipset
	//   - TipSetSelectors.Height(epoch, previous, anchor): tipset at the specified height
	//   - TipSetSelectors.Key(key): tipset with the specified key
	//
	// See types.TipSetSelector documentation for additional details.
	//
	// If the actor does not exist at the specified tipset, this function returns nil.
	//
	// Experimental: This API is experimental and may change without notice.
	StateGetActor(context.Context, address.Address, types.TipSetSelector) (*types.Actor, error) //perm:read

	// StateGetID retrieves the ID address for the specified address at the selected tipset.
	//
	// Every actor on the Filecoin network has a unique ID address (format: f0123).
	// This function resolves any address type (ID, robust, or delegated) to its canonical
	// ID address representation at the specified tipset.
	//
	// The function is particularly useful for:
	//   - Normalizing different address formats to a consistent representation
	//   - Following address changes across state transitions
	//   - Verifying that an address corresponds to an existing actor
	//
	// The TipSetSelector parameter provides flexible options for selecting the tipset.
	// See StateGetActor documentation for details on selection options.
	//
	// If the address cannot be resolved at the specified tipset, this function returns nil.
	//
	// Experimental: This API is experimental and may change without notice.
	StateGetID(context.Context, address.Address, types.TipSetSelector) (*address.Address, error) //perm:read

	// MethodGroup: Eth
	// These methods are used for Ethereum-compatible JSON-RPC calls
	//
	// ### Execution model reconciliation
	//
	// Ethereum relies on an immediate block-based execution model. The block that includes
	// a transaction is also the block that executes it. Each block specifies the state root
	// resulting from executing all transactions within it (output state).
	//
	// In Filecoin, at every epoch there is an unknown number of round winners all of whom are
	// entitled to publish a block. Blocks are collected into a tipset. A tipset is committed
	// only when the subsequent tipset is built on it (i.e. it becomes a parent). Block producers
	// execute the parent tipset and specify the resulting state root in the block being produced.
	// In other words, contrary to Ethereum, each block specifies the input state root.
	//
	// Ethereum clients expect transactions returned via eth_getBlock* to have a receipt
	// (due to immediate execution). For this reason:
	//
	//   - eth_blockNumber returns the latest executed epoch (head - 1)
	//   - The 'latest' block refers to the latest executed epoch (head - 1)
	//   - The 'pending' block refers to the current speculative tipset (head)
	//   - eth_getTransactionByHash returns the inclusion tipset of a message, but
	//     only after it has executed.
	//   - eth_getTransactionReceipt ditto.
	//
	// "Latest executed epoch" refers to the tipset that this node currently
	// accepts as the best parent tipset, based on the blocks it is accumulating
	// within the HEAD tipset.

	// EthFilecoinAPI methods

	// EthAddressToFilecoinAddress converts an Ethereum address to a Filecoin f410 address.
	EthAddressToFilecoinAddress(ctx context.Context, ethAddress ethtypes.EthAddress) (address.Address, error) //perm:read

	// FilecoinAddressToEthAddress converts any Filecoin address to an EthAddress.
	//
	// This method supports all Filecoin address types:
	// - "f0" and "f4" addresses: Converted directly.
	// - "f1", "f2", and "f3" addresses: First converted to their corresponding "f0" ID address, then to an EthAddress.
	//
	// Requirements:
	// - For "f1", "f2", and "f3" addresses, they must be instantiated on-chain, as "f0" ID addresses are only assigned to actors when they are created on-chain.
	// The simplest way to instantiate an address on chain is to send a transaction to the address.
	//
	// Note on chain reorganizations:
	// "f0" ID addresses are not permanent and can be affected by chain reorganizations. To account for this,
	// the API includes a `blkNum` parameter, which specifies the block number that is used to determine the tipset state to use for converting an
	// "f1"/"f2"/"f3" address to an "f0" address. This parameter functions similarly to the `blkNum` parameter in the existing `EthGetBlockByNumber` API.
	// See https://docs.alchemy.com/reference/eth-getblockbynumber for more details.
	//
	// Parameters:
	// - ctx: The context for the API call.
	// - filecoinAddress: The Filecoin address to convert.
	// - blkNum: The block number or state for the conversion. Defaults to "finalized" for maximum safety.
	//   Possible values: "pending", "latest", "finalized", "safe", or a specific block number represented as hex.
	//
	// Returns:
	// - The corresponding EthAddress.
	// - An error if the conversion fails.
	FilecoinAddressToEthAddress(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthAddress, error) //perm:read

	// EthBasicAPI methods

	// Web3ClientVersion returns the client version.
	// Maps to JSON-RPC method: "web3_clientVersion".
	Web3ClientVersion(ctx context.Context) (string, error) //perm:read

	// EthChainId retrieves the chain ID of the Ethereum-compatible network.
	// Maps to JSON-RPC method: "eth_chainId".
	EthChainId(ctx context.Context) (ethtypes.EthUint64, error) //perm:read

	// NetVersion retrieves the current network ID.
	// Maps to JSON-RPC method: "net_version".
	NetVersion(ctx context.Context) (string, error) //perm:read

	// NetListening checks if the node is actively listening for network connections.
	// Maps to JSON-RPC method: "net_listening".
	NetListening(ctx context.Context) (bool, error) //perm:read

	// EthProtocolVersion retrieves the Ethereum protocol version supported by the node.
	// Maps to JSON-RPC method: "eth_protocolVersion".
	EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error) //perm:read

	// EthSyncing checks if the node is currently syncing and returns the sync status.
	// Maps to JSON-RPC method: "eth_syncing".
	EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error) //perm:read

	// EthAccounts returns a list of Ethereum accounts managed by the node.
	// Maps to JSON-RPC method: "eth_accounts".
	// Lotus does not manage private keys, so this will always return an empty array.
	EthAccounts(ctx context.Context) ([]ethtypes.EthAddress, error) //perm:read

	// EthSendAPI methods

	// EthSendRawTransaction submits a raw Ethereum transaction to the network.
	// Maps to JSON-RPC method: "eth_sendRawTransaction".
	EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) //perm:read

	// EthSendRawTransactionUntrusted submits a raw Ethereum transaction from an untrusted source.
	EthSendRawTransactionUntrusted(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) //perm:read

	// EthTransactionAPI methods

	// EthBlockNumber returns the number of the latest executed block (head - 1).
	// Maps to JSON-RPC method: "eth_blockNumber".
	EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error) //perm:read

	// EthGetBlockTransactionCountByNumber returns the number of transactions in a block identified by
	// its block number or a special tag like "latest" or "finalized".
	// Maps to JSON-RPC method: "eth_getBlockTransactionCountByNumber".
	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum string) (ethtypes.EthUint64, error) //perm:read

	// EthGetBlockTransactionCountByHash returns the number of transactions in a block identified by
	// its block hash.
	// Maps to JSON-RPC method: "eth_getBlockTransactionCountByHash".
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error) //perm:read

	// EthGetBlockByHash retrieves a block by its hash. If fullTxInfo is true, it includes full
	// transaction objects; otherwise, it includes only transaction hashes.
	// Maps to JSON-RPC method: "eth_getBlockByHash".
	EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error) //perm:read

	// EthGetBlockByNumber retrieves a block by its number or a special tag like "latest" or
	// "finalized". If fullTxInfo is true, it includes full transaction objects; otherwise, it
	// includes only transaction hashes.
	// Maps to JSON-RPC method: "eth_getBlockByNumber".
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error) //perm:read

	// EthGetTransactionByHash retrieves a transaction by its hash.
	// Maps to JSON-RPC method: "eth_getTransactionByHash".
	EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error) //perm:read

	// EthGetTransactionByHashLimited retrieves a transaction by its hash, with an optional limit on
	// the chain epoch for state resolution.
	EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error) //perm:read

	// EthGetTransactionByBlockHashAndIndex retrieves a transaction by its block hash and index.
	// Maps to JSON-RPC method: "eth_getTransactionByBlockHashAndIndex".
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error) //perm:read

	// EthGetTransactionByBlockNumberAndIndex retrieves a transaction by its block number and index.
	// Maps to JSON-RPC method: "eth_getTransactionByBlockNumberAndIndex".
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum string, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error) //perm:read

	// EthGetMessageCidByTransactionHash retrieves the Filecoin CID corresponding to an Ethereum
	// transaction hash.
	// Maps to JSON-RPC method: "eth_getMessageCidByTransactionHash".
	EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error) //perm:read

	// EthGetTransactionHashByCid retrieves the Ethereum transaction hash corresponding to a Filecoin
	// CID.
	// Maps to JSON-RPC method: "eth_getTransactionHashByCid".
	EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error) //perm:read

	// EthGetTransactionCount retrieves the number of transactions sent from an address at a specific
	// block, identified by its number, hash, or a special tag like "latest" or "finalized".
	// Maps to JSON-RPC method: "eth_getTransactionCount".
	EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error) //perm:read

	// EthGetTransactionReceipt retrieves the receipt of a transaction by its hash.
	// Maps to JSON-RPC method: "eth_getTransactionReceipt".
	EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*ethtypes.EthTxReceipt, error) //perm:read

	// EthGetTransactionReceiptLimited retrieves the receipt of a transaction by its hash, with an
	// optional limit on the chain epoch for state resolution.
	EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTxReceipt, error) //perm:read

	// EthGetBlockReceipts retrieves all transaction receipts for a block identified by its number,
	// hash or a special tag like "latest" or "finalized".
	// Maps to JSON-RPC method: "eth_getBlockReceipts".
	EthGetBlockReceipts(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) ([]*ethtypes.EthTxReceipt, error) //perm:read

	// EthGetBlockReceiptsLimited retrieves all transaction receipts for a block identified by its
	// number, hash or a special tag like "latest" or "finalized", along with an optional limit on the
	// chain epoch for state resolution.
	EthGetBlockReceiptsLimited(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, limit abi.ChainEpoch) ([]*ethtypes.EthTxReceipt, error) //perm:read

	// EthLookupAPI methods

	// EthGetCode retrieves the contract code at a specific address and block state, identified by
	// its number, hash, or a special tag like "latest" or "finalized".
	// Maps to JSON-RPC method: "eth_getCode".
	EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) //perm:read

	// EthGetStorageAt retrieves the storage value at a specific position for a contract
	// at a given block state, identified by its number, hash, or a special tag like "latest" or
	// "finalized".
	// Maps to JSON-RPC method: "eth_getStorageAt".
	EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) //perm:read

	// EthGetBalance retrieves the balance of an Ethereum address at a specific block state,
	// identified by its number, hash, or a special tag like "latest" or "finalized".
	// Maps to JSON-RPC method: "eth_getBalance".
	EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error) //perm:read

	// EthTraceAPI methods

	// EthTraceBlock returns an OpenEthereum-compatible trace of the given block.
	// Maps to JSON-RPC method: "trace_block".
	//
	// Translates Filecoin semantics into Ethereum semantics and traces both EVM and FVM calls.
	//
	// Features:
	//
	// - FVM actor create events, calls, etc. show up as if they were EVM smart contract events.
	// - Native FVM call inputs are ABI-encoded (Solidity ABI) as if they were calls to a
	//   `handle_filecoin_method(uint64 method, uint64 codec, bytes params)` function
	//   (where `codec` is the IPLD codec of `params`).
	// - Native FVM call outputs (return values) are ABI-encoded as `(uint32 exit_code, uint64
	//   codec, bytes output)` where `codec` is the IPLD codec of `output`.
	//
	// Limitations (for now):
	//
	// 1. Block rewards are not included in the trace.
	// 2. SELFDESTRUCT operations are not included in the trace.
	// 3. EVM smart contract "create" events always specify `0xfe` as the "code" for newly created EVM
	//    smart contracts.
	EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error) //perm:read

	// EthTraceReplayBlockTransactions replays all transactions in a block returning the requested
	// traces for each transaction.
	// Maps to JSON-RPC method: "trace_replayBlockTransactions".
	//
	// The block can be identified by its number, hash or a special tag like "latest" or "finalized".
	// The `traceTypes` parameter allows filtering the traces based on their types.
	EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error) //perm:read

	// EthTraceTransaction returns an OpenEthereum-compatible trace of a specific transaction.
	// Maps to JSON-RPC method: "trace_transaction".
	EthTraceTransaction(ctx context.Context, txHash string) ([]*ethtypes.EthTraceTransaction, error) //perm:read

	// EthTraceFilter returns traces matching the given filter criteria.
	// Maps to JSON-RPC method: "trace_filter".
	EthTraceFilter(ctx context.Context, filter ethtypes.EthTraceFilterCriteria) ([]*ethtypes.EthTraceFilterResult, error) //perm:read

	// EthGasAPI methods

	// EthGasPrice retrieves the current gas price in the network.
	// Maps to JSON-RPC method: "eth_gasPrice".
	EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error) //perm:read

	// EthFeeHistory retrieves historical gas fee data for a range of blocks.
	// Maps to JSON-RPC method: "eth_feeHistory".
	EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error) //perm:read

	// EthMaxPriorityFeePerGas retrieves the maximum priority fee per gas in the network.
	// Maps to JSON-RPC method: "eth_maxPriorityFeePerGas".
	EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error) //perm:read

	// EthEstimateGas estimates the gas required to execute a transaction.
	// Maps to JSON-RPC method: "eth_estimateGas".
	EthEstimateGas(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthUint64, error) //perm:read

	// EthCall executes a read-only call to a contract at a specific block state, identified by
	// its number, hash, or a special tag like "latest" or "finalized".
	// Maps to JSON-RPC method: "eth_call".
	EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) //perm:read

	// EthEventsAPI methods

	// EthGetLogs retrieves event logs matching given filter specification.
	// Maps to JSON-RPC method: "eth_getLogs".
	EthGetLogs(ctx context.Context, filter *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error) //perm:read

	// EthNewBlockFilter installs a persistent filter to notify when a new block arrives.
	// Maps to JSON-RPC method: "eth_newBlockFilter".
	EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error) //perm:read

	// EthNewPendingTransactionFilter installs a persistent filter to notify when new messages arrive
	// in the message pool.
	// Maps to JSON-RPC method: "eth_newPendingTransactionFilter".
	EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error) //perm:read

	// EthNewFilter installs a persistent filter based on given filter specification.
	// Maps to JSON-RPC method: "eth_newFilter".
	EthNewFilter(ctx context.Context, filter *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error) //perm:read

	// EthUninstallFilter uninstalls a filter with given id.
	// Maps to JSON-RPC method: "eth_uninstallFilter".
	EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error) //perm:read

	// EthGetFilterChanges retrieves event logs that occurred since the last poll for a filter.
	// Maps to JSON-RPC method: "eth_getFilterChanges".
	EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) //perm:read

	// EthGetFilterLogs retrieves event logs matching filter with given id.
	// Maps to JSON-RPC method: "eth_getFilterLogs".
	EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) //perm:read

	// EthSubscribe subscribes to different event types using websockets.
	// Maps to JSON-RPC method: "eth_subscribe".
	//
	// eventTypes is one or more of:
	//  - newHeads: notify when new blocks arrive.
	//  - pendingTransactions: notify when new messages arrive in the message pool.
	//  - logs: notify new event logs that match a criteria
	//
	// params contains additional parameters used with the log event type
	// The client will receive a stream of EthSubscriptionResponse values until EthUnsubscribe is called.
	EthSubscribe(ctx context.Context, params jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error) //perm:read

	// EthUnsubscribe unsubscribes from a websocket subscription.
	// Maps to JSON-RPC method: "eth_unsubscribe".
	EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error) //perm:read
}
