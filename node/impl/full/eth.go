package full

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/evm"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
	builtinevm "github.com/filecoin-project/lotus/chain/actors/builtin/evm"
	"github.com/filecoin-project/lotus/chain/ethhashlookup"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var ErrUnsupported = errors.New("unsupported method")

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
	EthEstimateGas(ctx context.Context, tx ethtypes.EthCall) (ethtypes.EthUint64, error)
	EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error)
	EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error)
	Web3ClientVersion(ctx context.Context) (string, error)
	EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error)
	EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error)
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

	_ EthModuleAPI = *new(api.Gateway)
)

// EthModule provides the default implementation of the standard Ethereum JSON-RPC API.
//
// # Execution model reconciliation
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
type EthModule struct {
	Chain            *store.ChainStore
	Mpool            *messagepool.MessagePool
	StateManager     *stmgr.StateManager
	EthTxHashManager *EthTxHashManager

	ChainAPI
	MpoolAPI
	StateAPI
	SyncAPI
}

var _ EthModuleAPI = (*EthModule)(nil)

type EthEvent struct {
	Chain                *store.ChainStore
	EventFilterManager   *filter.EventFilterManager
	TipSetFilterManager  *filter.TipSetFilterManager
	MemPoolFilterManager *filter.MemPoolFilterManager
	FilterStore          filter.FilterStore
	SubManager           *EthSubscriptionManager
	MaxFilterHeightRange abi.ChainEpoch
	SubscribtionCtx      context.Context
}

var _ EthEventAPI = (*EthEvent)(nil)

type EthAPI struct {
	fx.In

	Chain *store.ChainStore

	EthModuleAPI
	EthEventAPI
}

var ErrNullRound = errors.New("requested epoch was a null round")

func (a *EthModule) StateNetworkName(ctx context.Context) (dtypes.NetworkName, error) {
	return stmgr.GetNetworkName(ctx, a.StateManager, a.Chain.GetHeaviestTipSet().ParentState())
}

func (a *EthModule) EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error) {
	// eth_blockNumber needs to return the height of the latest committed tipset.
	// Ethereum clients expect all transactions included in this block to have execution outputs.
	// This is the parent of the head tipset. The head tipset is speculative, has not been
	// recognized by the network, and its messages are only included, not executed.
	// See https://github.com/filecoin-project/ref-fvm/issues/1135.
	heaviest := a.Chain.GetHeaviestTipSet()
	if height := heaviest.Height(); height == 0 {
		// we're at genesis.
		return ethtypes.EthUint64(height), nil
	}
	// First non-null parent.
	effectiveParent := heaviest.Parents()
	parent, err := a.Chain.GetTipSetFromKey(ctx, effectiveParent)
	if err != nil {
		return 0, err
	}
	return ethtypes.EthUint64(parent.Height()), nil
}

func (a *EthModule) EthAccounts(context.Context) ([]ethtypes.EthAddress, error) {
	// The lotus node is not expected to hold manage accounts, so we'll always return an empty array
	return []ethtypes.EthAddress{}, nil
}

func (a *EthAPI) EthAddressToFilecoinAddress(ctx context.Context, ethAddress ethtypes.EthAddress) (address.Address, error) {
	return ethAddress.ToFilecoinAddress()
}

func (a *EthAPI) FilecoinAddressToEthAddress(ctx context.Context, filecoinAddress address.Address) (ethtypes.EthAddress, error) {
	return ethtypes.EthAddressFromFilecoinAddress(filecoinAddress)
}

func (a *EthModule) countTipsetMsgs(ctx context.Context, ts *types.TipSet) (int, error) {
	blkMsgs, err := a.Chain.BlockMsgsForTipset(ctx, ts)
	if err != nil {
		return 0, xerrors.Errorf("error loading messages for tipset: %v: %w", ts, err)
	}

	count := 0
	for _, blkMsg := range blkMsgs {
		// TODO: may need to run canonical ordering and deduplication here
		count += len(blkMsg.BlsMessages) + len(blkMsg.SecpkMessages)
	}
	return count, nil
}

func (a *EthModule) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum ethtypes.EthUint64) (ethtypes.EthUint64, error) {
	ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(blkNum), nil, false)
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}

	count, err := a.countTipsetMsgs(ctx, ts)
	return ethtypes.EthUint64(count), err
}

func (a *EthModule) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error) {
	ts, err := a.Chain.GetTipSetByCid(ctx, blkHash.ToCid())
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	count, err := a.countTipsetMsgs(ctx, ts)
	return ethtypes.EthUint64(count), err
}

func (a *EthModule) EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error) {
	ts, err := a.Chain.GetTipSetByCid(ctx, blkHash.ToCid())
	if err != nil {
		return ethtypes.EthBlock{}, xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	return newEthBlockFromFilecoinTipSet(ctx, ts, fullTxInfo, a.Chain, a.StateAPI)
}

func (a *EthModule) EthGetBlockByNumber(ctx context.Context, blkParam string, fullTxInfo bool) (ethtypes.EthBlock, error) {
	ts, err := getTipsetByBlockNumber(ctx, a.Chain, blkParam, true)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	return newEthBlockFromFilecoinTipSet(ctx, ts, fullTxInfo, a.Chain, a.StateAPI)
}

func (a *EthModule) EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error) {
	return a.EthGetTransactionByHashLimited(ctx, txHash, api.LookbackNoLimit)

}

func (a *EthModule) EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error) {
	// Ethereum's behavior is to return null when the txHash is invalid, so we use nil to check if txHash is valid
	if txHash == nil {
		return nil, nil
	}

	c, err := a.EthTxHashManager.TransactionHashLookup.GetCidFromHash(*txHash)
	if err != nil {
		log.Debug("could not find transaction hash %s in lookup table", txHash.String())
	}

	// This isn't an eth transaction we have the mapping for, so let's look it up as a filecoin message
	if c == cid.Undef {
		c = txHash.ToCid()
	}

	// first, try to get the cid from mined transactions
	msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, c, limit, true)
	if err == nil && msgLookup != nil {
		tx, err := newEthTxFromMessageLookup(ctx, msgLookup, -1, a.Chain, a.StateAPI)
		if err == nil {
			return &tx, nil
		}
	}

	// if not found, try to get it from the mempool
	pending, err := a.MpoolAPI.MpoolPending(ctx, types.EmptyTSK)
	if err != nil {
		// inability to fetch mpool pending transactions is an internal node error
		// that needs to be reported as-is
		return nil, fmt.Errorf("cannot get pending txs from mpool: %s", err)
	}

	for _, p := range pending {
		if p.Cid() == c {
			tx, err := newEthTxFromSignedMessage(ctx, p, a.StateAPI)
			if err != nil {
				return nil, fmt.Errorf("could not convert Filecoin message into tx: %s", err)
			}
			return &tx, nil
		}
	}
	// Ethereum clients expect an empty response when the message was not found
	return nil, nil
}

func (a *EthModule) EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error) {
	// Ethereum's behavior is to return null when the txHash is invalid, so we use nil to check if txHash is valid
	if txHash == nil {
		return nil, nil
	}

	c, err := a.EthTxHashManager.TransactionHashLookup.GetCidFromHash(*txHash)
	// We fall out of the first condition and continue
	if errors.Is(err, ethhashlookup.ErrNotFound) {
		log.Debug("could not find transaction hash %s in lookup table", txHash.String())
	} else if err != nil {
		return nil, xerrors.Errorf("database error: %w", err)
	} else {
		return &c, nil
	}

	// This isn't an eth transaction we have the mapping for, so let's try looking it up as a filecoin message
	if c == cid.Undef {
		c = txHash.ToCid()
	}

	_, err = a.Chain.GetSignedMessage(ctx, c)
	if err == nil {
		// This is an Eth Tx, Secp message, Or BLS message in the mpool
		return &c, nil
	}

	_, err = a.Chain.GetMessage(ctx, c)
	if err == nil {
		// This is a BLS message
		return &c, nil
	}

	// Ethereum clients expect an empty response when the message was not found
	return nil, nil
}

func (a *EthModule) EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error) {
	hash, err := ethTxHashFromMessageCid(ctx, cid, a.StateAPI)
	if hash == ethtypes.EmptyEthHash {
		// not found
		return nil, nil
	}

	return &hash, err
}

func (a *EthModule) EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error) {
	addr, err := sender.ToFilecoinAddress()
	if err != nil {
		return ethtypes.EthUint64(0), nil
	}

	ts, err := getTipsetByEthBlockNumberOrHash(ctx, a.Chain, blkParam)
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("failed to process block param: %v; %w", blkParam, err)
	}

	// First, handle the case where the "sender" is an EVM actor.
	if actor, err := a.StateManager.LoadActor(ctx, addr, ts); err != nil {
		if xerrors.Is(err, types.ErrActorNotFound) {
			return 0, nil
		}
		return 0, xerrors.Errorf("failed to lookup contract %s: %w", sender, err)
	} else if builtinactors.IsEvmActor(actor.Code) {
		evmState, err := builtinevm.Load(a.Chain.ActorStore(ctx), actor)
		if err != nil {
			return 0, xerrors.Errorf("failed to load evm state: %w", err)
		}
		if alive, err := evmState.IsAlive(); err != nil {
			return 0, err
		} else if !alive {
			return 0, nil
		}
		nonce, err := evmState.Nonce()
		return ethtypes.EthUint64(nonce), err
	}

	nonce, err := a.Mpool.GetNonce(ctx, addr, ts.Key())
	if err != nil {
		return ethtypes.EthUint64(0), nil
	}
	return ethtypes.EthUint64(nonce), nil
}

func (a *EthModule) EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error) {
	return a.EthGetTransactionReceiptLimited(ctx, txHash, api.LookbackNoLimit)
}

func (a *EthModule) EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*api.EthTxReceipt, error) {
	c, err := a.EthTxHashManager.TransactionHashLookup.GetCidFromHash(txHash)
	if err != nil {
		log.Debug("could not find transaction hash %s in lookup table", txHash.String())
	}

	// This isn't an eth transaction we have the mapping for, so let's look it up as a filecoin message
	if c == cid.Undef {
		c = txHash.ToCid()
	}

	msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, c, limit, true)
	if err != nil || msgLookup == nil {
		return nil, nil
	}

	tx, err := newEthTxFromMessageLookup(ctx, msgLookup, -1, a.Chain, a.StateAPI)
	if err != nil {
		return nil, nil
	}

	var events []types.Event
	if rct := msgLookup.Receipt; rct.EventsRoot != nil {
		events, err = a.ChainAPI.ChainGetEvents(ctx, *rct.EventsRoot)
		if err != nil {
			return nil, nil
		}
	}

	receipt, err := newEthTxReceipt(ctx, tx, msgLookup, events, a.Chain, a.StateAPI)
	if err != nil {
		return nil, nil
	}

	return &receipt, nil
}

func (a *EthAPI) EthGetTransactionByBlockHashAndIndex(context.Context, ethtypes.EthHash, ethtypes.EthUint64) (ethtypes.EthTx, error) {
	return ethtypes.EthTx{}, ErrUnsupported
}

func (a *EthAPI) EthGetTransactionByBlockNumberAndIndex(context.Context, ethtypes.EthUint64, ethtypes.EthUint64) (ethtypes.EthTx, error) {
	return ethtypes.EthTx{}, ErrUnsupported
}

// EthGetCode returns string value of the compiled bytecode
func (a *EthModule) EthGetCode(ctx context.Context, ethAddr ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	to, err := ethAddr.ToFilecoinAddress()
	if err != nil {
		return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
	}

	ts, err := getTipsetByEthBlockNumberOrHash(ctx, a.Chain, blkParam)
	if err != nil {
		return nil, xerrors.Errorf("failed to process block param: %v; %w", blkParam, err)
	}

	// StateManager.Call will panic if there is no parent
	if ts.Height() == 0 {
		return nil, xerrors.Errorf("block param must not specify genesis block")
	}

	actor, err := a.StateManager.LoadActor(ctx, to, ts)
	if err != nil {
		if xerrors.Is(err, types.ErrActorNotFound) {
			return nil, nil
		}
		return nil, xerrors.Errorf("failed to lookup contract %s: %w", ethAddr, err)
	}

	// Not a contract. We could try to distinguish between accounts and "native" contracts here,
	// but it's not worth it.
	if !builtinactors.IsEvmActor(actor.Code) {
		return nil, nil
	}

	msg := &types.Message{
		From:       builtinactors.SystemActorAddr,
		To:         to,
		Value:      big.Zero(),
		Method:     builtintypes.MethodsEVM.GetBytecode,
		Params:     nil,
		GasLimit:   build.BlockGasLimit,
		GasFeeCap:  big.Zero(),
		GasPremium: big.Zero(),
	}

	// Try calling until we find a height with no migration.
	var res *api.InvocResult
	for {
		res, err = a.StateManager.Call(ctx, msg, ts)
		if err != stmgr.ErrExpensiveFork {
			break
		}
		ts, err = a.Chain.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("getting parent tipset: %w", err)
		}
	}

	if err != nil {
		return nil, xerrors.Errorf("failed to call GetBytecode: %w", err)
	}

	if res.MsgRct == nil {
		return nil, fmt.Errorf("no message receipt")
	}

	if res.MsgRct.ExitCode.IsError() {
		return nil, xerrors.Errorf("GetBytecode failed: %s", res.Error)
	}

	var getBytecodeReturn evm.GetBytecodeReturn
	if err := getBytecodeReturn.UnmarshalCBOR(bytes.NewReader(res.MsgRct.Return)); err != nil {
		return nil, fmt.Errorf("failed to decode EVM bytecode CID: %w", err)
	}

	// The contract has selfdestructed, so the code is "empty".
	if getBytecodeReturn.Cid == nil {
		return nil, nil
	}

	blk, err := a.Chain.StateBlockstore().Get(ctx, *getBytecodeReturn.Cid)
	if err != nil {
		return nil, fmt.Errorf("failed to get EVM bytecode: %w", err)
	}

	return blk.RawData(), nil
}

func (a *EthModule) EthGetStorageAt(ctx context.Context, ethAddr ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	ts, err := getTipsetByEthBlockNumberOrHash(ctx, a.Chain, blkParam)
	if err != nil {
		return nil, xerrors.Errorf("failed to process block param: %v; %w", blkParam, err)
	}

	l := len(position)
	if l > 32 {
		return nil, fmt.Errorf("supplied storage key is too long")
	}

	// pad with zero bytes if smaller than 32 bytes
	position = append(make([]byte, 32-l, 32), position...)

	to, err := ethAddr.ToFilecoinAddress()
	if err != nil {
		return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
	}

	// use the system actor as the caller
	from, err := address.NewIDAddress(0)
	if err != nil {
		return nil, fmt.Errorf("failed to construct system sender address: %w", err)
	}

	actor, err := a.StateManager.LoadActor(ctx, to, ts)
	if err != nil {
		if xerrors.Is(err, types.ErrActorNotFound) {
			return ethtypes.EthBytes(make([]byte, 32)), nil
		}
		return nil, xerrors.Errorf("failed to lookup contract %s: %w", ethAddr, err)
	}

	if !builtinactors.IsEvmActor(actor.Code) {
		return ethtypes.EthBytes(make([]byte, 32)), nil
	}

	params, err := actors.SerializeParams(&evm.GetStorageAtParams{
		StorageKey: *(*[32]byte)(position),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to serialize parameters: %w", err)
	}

	msg := &types.Message{
		From:       from,
		To:         to,
		Value:      big.Zero(),
		Method:     builtintypes.MethodsEVM.GetStorageAt,
		Params:     params,
		GasLimit:   build.BlockGasLimit,
		GasFeeCap:  big.Zero(),
		GasPremium: big.Zero(),
	}

	// Try calling until we find a height with no migration.
	var res *api.InvocResult
	for {
		res, err = a.StateManager.Call(ctx, msg, ts)
		if err != stmgr.ErrExpensiveFork {
			break
		}
		ts, err = a.Chain.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("getting parent tipset: %w", err)
		}
	}

	if err != nil {
		return nil, xerrors.Errorf("Call failed: %w", err)
	}

	if res.MsgRct == nil {
		return nil, xerrors.Errorf("no message receipt")
	}

	if res.MsgRct.ExitCode.IsError() {
		return nil, xerrors.Errorf("failed to lookup storage slot: %s", res.Error)
	}

	var ret abi.CborBytes
	if err := ret.UnmarshalCBOR(bytes.NewReader(res.MsgRct.Return)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal storage slot: %w", err)
	}

	// pad with zero bytes if smaller than 32 bytes
	ret = append(make([]byte, 32-len(ret), 32), ret...)

	return ethtypes.EthBytes(ret), nil
}

func (a *EthModule) EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error) {
	filAddr, err := address.ToFilecoinAddress()
	if err != nil {
		return ethtypes.EthBigInt{}, err
	}

	ts, err := getTipsetByEthBlockNumberOrHash(ctx, a.Chain, blkParam)
	if err != nil {
		return ethtypes.EthBigInt{}, xerrors.Errorf("failed to process block param: %v; %w", blkParam, err)
	}

	st, _, err := a.StateManager.TipSetState(ctx, ts)
	if err != nil {
		return ethtypes.EthBigInt{}, xerrors.Errorf("failed to compute tipset state: %w", err)
	}

	actor, err := a.StateManager.LoadActorRaw(ctx, filAddr, st)
	if xerrors.Is(err, types.ErrActorNotFound) {
		return ethtypes.EthBigIntZero, nil
	} else if err != nil {
		return ethtypes.EthBigInt{}, err
	}

	return ethtypes.EthBigInt{Int: actor.Balance.Int}, nil
}

func (a *EthModule) EthChainId(ctx context.Context) (ethtypes.EthUint64, error) {
	return ethtypes.EthUint64(build.Eip155ChainId), nil
}

func (a *EthModule) EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error) {
	state, err := a.SyncAPI.SyncState(ctx)
	if err != nil {
		return ethtypes.EthSyncingResult{}, fmt.Errorf("failed calling SyncState: %w", err)
	}

	if len(state.ActiveSyncs) == 0 {
		return ethtypes.EthSyncingResult{}, errors.New("no active syncs, try again")
	}

	working := -1
	for i, ss := range state.ActiveSyncs {
		if ss.Stage == api.StageIdle {
			continue
		}
		working = i
	}
	if working == -1 {
		working = len(state.ActiveSyncs) - 1
	}

	ss := state.ActiveSyncs[working]
	if ss.Base == nil || ss.Target == nil {
		return ethtypes.EthSyncingResult{}, errors.New("missing syncing information, try again")
	}

	res := ethtypes.EthSyncingResult{
		DoneSync:      ss.Stage == api.StageSyncComplete,
		CurrentBlock:  ethtypes.EthUint64(ss.Height),
		StartingBlock: ethtypes.EthUint64(ss.Base.Height()),
		HighestBlock:  ethtypes.EthUint64(ss.Target.Height()),
	}

	return res, nil
}

func (a *EthModule) EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error) {
	params, err := jsonrpc.DecodeParams[ethtypes.EthFeeHistoryParams](p)
	if err != nil {
		return ethtypes.EthFeeHistory{}, xerrors.Errorf("decoding params: %w", err)
	}
	if params.BlkCount > 1024 {
		return ethtypes.EthFeeHistory{}, fmt.Errorf("block count should be smaller than 1024")
	}
	rewardPercentiles := make([]float64, 0)
	if params.RewardPercentiles != nil {
		rewardPercentiles = append(rewardPercentiles, *params.RewardPercentiles...)
	}
	for i, rp := range rewardPercentiles {
		if rp < 0 || rp > 100 {
			return ethtypes.EthFeeHistory{}, fmt.Errorf("invalid reward percentile: %f should be between 0 and 100", rp)
		}
		if i > 0 && rp < rewardPercentiles[i-1] {
			return ethtypes.EthFeeHistory{}, fmt.Errorf("invalid reward percentile: %f should be larger than %f", rp, rewardPercentiles[i-1])
		}
	}

	ts, err := getTipsetByBlockNumber(ctx, a.Chain, params.NewestBlkNum, false)
	if err != nil {
		return ethtypes.EthFeeHistory{}, fmt.Errorf("bad block parameter %s: %s", params.NewestBlkNum, err)
	}

	var (
		basefee         = ts.Blocks()[0].ParentBaseFee
		oldestBlkHeight = uint64(1)

		// NOTE: baseFeePerGas should include the next block after the newest of the returned range,
		//  because the next base fee can be inferred from the messages in the newest block.
		//  However, this is NOT the case in Filecoin due to deferred execution, so the best
		//  we can do is duplicate the last value.
		baseFeeArray      = []ethtypes.EthBigInt{ethtypes.EthBigInt(basefee)}
		rewardsArray      = make([][]ethtypes.EthBigInt, 0)
		gasUsedRatioArray = []float64{}
		blocksIncluded    int
	)

	for blocksIncluded < int(params.BlkCount) && ts.Height() > 0 {
		msgs, rcpts, err := messagesAndReceipts(ctx, ts, a.Chain, a.StateAPI)
		if err != nil {
			return ethtypes.EthFeeHistory{}, xerrors.Errorf("failed to retrieve messages and receipts for height %d: %w", ts.Height(), err)
		}

		txGasRewards := gasRewardSorter{}
		for i, msg := range msgs {
			effectivePremium := msg.VMMessage().EffectiveGasPremium(basefee)
			txGasRewards = append(txGasRewards, gasRewardTuple{
				premium: effectivePremium,
				gasUsed: rcpts[i].GasUsed,
			})
		}

		rewards, totalGasUsed := calculateRewardsAndGasUsed(rewardPercentiles, txGasRewards)
		maxGas := build.BlockGasLimit * int64(len(ts.Blocks()))

		// arrays should be reversed at the end
		baseFeeArray = append(baseFeeArray, ethtypes.EthBigInt(basefee))
		gasUsedRatioArray = append(gasUsedRatioArray, float64(totalGasUsed)/float64(maxGas))
		rewardsArray = append(rewardsArray, rewards)
		oldestBlkHeight = uint64(ts.Height())
		blocksIncluded++

		parentTsKey := ts.Parents()
		ts, err = a.Chain.LoadTipSet(ctx, parentTsKey)
		if err != nil {
			return ethtypes.EthFeeHistory{}, fmt.Errorf("cannot load tipset key: %v", parentTsKey)
		}
	}

	// Reverse the arrays; we collected them newest to oldest; the client expects oldest to newest.
	for i, j := 0, len(baseFeeArray)-1; i < j; i, j = i+1, j-1 {
		baseFeeArray[i], baseFeeArray[j] = baseFeeArray[j], baseFeeArray[i]
	}
	for i, j := 0, len(gasUsedRatioArray)-1; i < j; i, j = i+1, j-1 {
		gasUsedRatioArray[i], gasUsedRatioArray[j] = gasUsedRatioArray[j], gasUsedRatioArray[i]
	}
	for i, j := 0, len(rewardsArray)-1; i < j; i, j = i+1, j-1 {
		rewardsArray[i], rewardsArray[j] = rewardsArray[j], rewardsArray[i]
	}

	ret := ethtypes.EthFeeHistory{
		OldestBlock:   ethtypes.EthUint64(oldestBlkHeight),
		BaseFeePerGas: baseFeeArray,
		GasUsedRatio:  gasUsedRatioArray,
	}
	if params.RewardPercentiles != nil {
		ret.Reward = &rewardsArray
	}
	return ret, nil
}

func (a *EthModule) NetVersion(_ context.Context) (string, error) {
	return strconv.FormatInt(build.Eip155ChainId, 10), nil
}

func (a *EthModule) NetListening(ctx context.Context) (bool, error) {
	return true, nil
}

func (a *EthModule) EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error) {
	height := a.Chain.GetHeaviestTipSet().Height()
	return ethtypes.EthUint64(a.StateManager.GetNetworkVersion(ctx, height)), nil
}

func (a *EthModule) EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error) {
	gasPremium, err := a.GasAPI.GasEstimateGasPremium(ctx, 0, builtinactors.SystemActorAddr, 10000, types.EmptyTSK)
	if err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}
	return ethtypes.EthBigInt(gasPremium), nil
}

func (a *EthModule) EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error) {
	// According to Geth's implementation, eth_gasPrice should return base + tip
	// Ref: https://github.com/ethereum/pm/issues/328#issuecomment-853234014

	ts := a.Chain.GetHeaviestTipSet()
	baseFee := ts.Blocks()[0].ParentBaseFee

	premium, err := a.EthMaxPriorityFeePerGas(ctx)
	if err != nil {
		return ethtypes.EthBigInt(big.Zero()), nil
	}

	gasPrice := big.Add(baseFee, big.Int(premium))
	return ethtypes.EthBigInt(gasPrice), nil
}

func (a *EthModule) EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	txArgs, err := ethtypes.ParseEthTxArgs(rawTx)
	if err != nil {
		return ethtypes.EmptyEthHash, err
	}

	smsg, err := txArgs.ToSignedMessage()
	if err != nil {
		return ethtypes.EmptyEthHash, err
	}

	_, err = a.MpoolAPI.MpoolPush(ctx, smsg)
	if err != nil {
		return ethtypes.EmptyEthHash, err
	}

	return ethtypes.EthHashFromTxBytes(rawTx), nil
}

func (a *EthModule) Web3ClientVersion(ctx context.Context) (string, error) {
	return build.UserVersion(), nil
}

func (a *EthModule) EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error) {
	ts, err := getTipsetByBlockNumber(ctx, a.Chain, blkNum, false)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset: %w", err)
	}

	_, trace, err := a.StateManager.ExecutionTrace(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed when calling ExecutionTrace: %w", err)
	}

	tsParent, err := a.ChainAPI.ChainGetTipSetByHeight(ctx, ts.Height()+1, a.Chain.GetHeaviestTipSet().Key())
	if err != nil {
		return nil, xerrors.Errorf("cannot get tipset at height: %v", ts.Height()+1)
	}

	msgs, err := a.ChainGetParentMessages(ctx, tsParent.Blocks()[0].Cid())
	if err != nil {
		return nil, xerrors.Errorf("failed to get parent messages: %w", err)
	}

	cid, err := ts.Key().Cid()
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset key cid: %w", err)
	}

	blkHash, err := ethtypes.EthHashFromCid(cid)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse eth hash from cid: %w", err)
	}

	allTraces := make([]*ethtypes.EthTraceBlock, 0, len(trace))
	msgIdx := 0
	for _, ir := range trace {
		// ignore messages from system actor
		if ir.Msg.From == builtinactors.SystemActorAddr {
			continue
		}

		// as we include TransactionPosition in the results, lets do sanity checking that the
		// traces are indeed in the message execution order
		if ir.Msg.Cid() != msgs[msgIdx].Message.Cid() {
			return nil, xerrors.Errorf("traces are not in message execution order")
		}
		msgIdx++

		txHash, err := a.EthGetTransactionHashByCid(ctx, ir.MsgCid)
		if err != nil {
			return nil, xerrors.Errorf("failed to get transaction hash by cid: %w", err)
		}
		if txHash == nil {
			log.Warnf("cannot find transaction hash for cid %s", ir.MsgCid)
			continue
		}

		traces := []*ethtypes.EthTrace{}
		err = buildTraces(ctx, &traces, nil, []int{}, ir.ExecutionTrace, int64(ts.Height()), a.StateAPI)
		if err != nil {
			return nil, xerrors.Errorf("failed building traces: %w", err)
		}

		traceBlocks := make([]*ethtypes.EthTraceBlock, 0, len(traces))
		for _, trace := range traces {
			traceBlocks = append(traceBlocks, &ethtypes.EthTraceBlock{
				EthTrace:            trace,
				BlockHash:           blkHash,
				BlockNumber:         int64(ts.Height()),
				TransactionHash:     *txHash,
				TransactionPosition: msgIdx,
			})
		}

		allTraces = append(allTraces, traceBlocks...)
	}

	return allTraces, nil
}

func (a *EthModule) EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error) {
	if len(traceTypes) != 1 || traceTypes[0] != "trace" {
		return nil, fmt.Errorf("only 'trace' is supported")
	}

	ts, err := getTipsetByBlockNumber(ctx, a.Chain, blkNum, false)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tipset: %w", err)
	}

	_, trace, err := a.StateManager.ExecutionTrace(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed when calling ExecutionTrace: %w", err)
	}

	allTraces := make([]*ethtypes.EthTraceReplayBlockTransaction, 0, len(trace))
	for _, ir := range trace {
		// ignore messages from system actor
		if ir.Msg.From == builtinactors.SystemActorAddr {
			continue
		}

		txHash, err := a.EthGetTransactionHashByCid(ctx, ir.MsgCid)
		if err != nil {
			return nil, xerrors.Errorf("failed to get transaction hash by cid: %w", err)
		}
		if txHash == nil {
			log.Warnf("cannot find transaction hash for cid %s", ir.MsgCid)
			continue
		}

		var output ethtypes.EthBytes
		invokeCreateOnEAM := ir.Msg.To == builtin.EthereumAddressManagerActorAddr && (ir.Msg.Method == builtin.MethodsEAM.Create || ir.Msg.Method == builtin.MethodsEAM.Create2)
		if ir.Msg.Method == builtin.MethodsEVM.InvokeContract || invokeCreateOnEAM {
			output, err = decodePayload(ir.ExecutionTrace.MsgRct.Return, ir.ExecutionTrace.MsgRct.ReturnCodec)
			if err != nil {
				return nil, xerrors.Errorf("failed to decode payload: %w", err)
			}
		} else {
			output, err = handleFilecoinMethodOutput(ir.ExecutionTrace.MsgRct.ExitCode, ir.ExecutionTrace.MsgRct.ReturnCodec, ir.ExecutionTrace.MsgRct.Return)
			if err != nil {
				return nil, xerrors.Errorf("could not convert output: %w", err)
			}
		}

		t := ethtypes.EthTraceReplayBlockTransaction{
			Output:          output,
			TransactionHash: *txHash,
			StateDiff:       nil,
			VmTrace:         nil,
		}

		err = buildTraces(ctx, &t.Trace, nil, []int{}, ir.ExecutionTrace, int64(ts.Height()), a.StateAPI)
		if err != nil {
			return nil, xerrors.Errorf("failed building traces: %w", err)
		}

		allTraces = append(allTraces, &t)
	}

	return allTraces, nil
}

func (a *EthModule) applyMessage(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (res *api.InvocResult, err error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("cannot get tipset: %w", err)
	}

	applyTsMessages := true
	if os.Getenv("LOTUS_SKIP_APPLY_TS_MESSAGE_CALL_WITH_GAS") == "1" {
		applyTsMessages = false
	}

	// Try calling until we find a height with no migration.
	for {
		res, err = a.StateManager.CallWithGas(ctx, msg, []types.ChainMsg{}, ts, applyTsMessages)
		if err != stmgr.ErrExpensiveFork {
			break
		}
		ts, err = a.Chain.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("getting parent tipset: %w", err)
		}
	}
	if err != nil {
		return nil, xerrors.Errorf("CallWithGas failed: %w", err)
	}
	if res.MsgRct.ExitCode.IsError() {
		reason := parseEthRevert(res.MsgRct.Return)
		return nil, xerrors.Errorf("message execution failed: exit %s, revert reason: %s, vm error: %s", res.MsgRct.ExitCode, reason, res.Error)
	}
	return res, nil
}

func (a *EthModule) EthEstimateGas(ctx context.Context, tx ethtypes.EthCall) (ethtypes.EthUint64, error) {
	msg, err := ethCallToFilecoinMessage(ctx, tx)
	if err != nil {
		return ethtypes.EthUint64(0), err
	}

	// Set the gas limit to the zero sentinel value, which makes
	// gas estimation actually run.
	msg.GasLimit = 0

	ts := a.Chain.GetHeaviestTipSet()
	gassedMsg, err := a.GasAPI.GasEstimateMessageGas(ctx, msg, nil, ts.Key())
	if err != nil {
		// On failure, GasEstimateMessageGas doesn't actually return the invocation result,
		// it just returns an error. That means we can't get the revert reason.
		//
		// So we re-execute the message with EthCall (well, applyMessage which contains the
		// guts of EthCall). This will give us an ethereum specific error with revert
		// information.
		msg.GasLimit = build.BlockGasLimit
		if _, err2 := a.applyMessage(ctx, msg, ts.Key()); err2 != nil {
			err = err2
		}
		return ethtypes.EthUint64(0), xerrors.Errorf("failed to estimate gas: %w", err)
	}

	expectedGas, err := ethGasSearch(ctx, a.Chain, a.Stmgr, a.Mpool, gassedMsg, ts)
	if err != nil {
		return 0, xerrors.Errorf("gas search failed: %w", err)
	}

	return ethtypes.EthUint64(expectedGas), nil
}

// gasSearch does an exponential search to find a gas value to execute the
// message with. It first finds a high gas limit that allows the message to execute
// by doubling the previous gas limit until it succeeds then does a binary
// search till it gets within a range of 1%
func gasSearch(
	ctx context.Context,
	smgr *stmgr.StateManager,
	msgIn *types.Message,
	priorMsgs []types.ChainMsg,
	ts *types.TipSet,
) (int64, error) {
	msg := *msgIn

	high := msg.GasLimit
	low := msg.GasLimit

	applyTsMessages := true
	if os.Getenv("LOTUS_SKIP_APPLY_TS_MESSAGE_CALL_WITH_GAS") == "1" {
		applyTsMessages = false
	}

	canSucceed := func(limit int64) (bool, error) {
		msg.GasLimit = limit

		res, err := smgr.CallWithGas(ctx, &msg, priorMsgs, ts, applyTsMessages)
		if err != nil {
			return false, xerrors.Errorf("CallWithGas failed: %w", err)
		}

		if res.MsgRct.ExitCode.IsSuccess() {
			return true, nil
		}

		return false, nil
	}

	for {
		ok, err := canSucceed(high)
		if err != nil {
			return -1, xerrors.Errorf("searching for high gas limit failed: %w", err)
		}
		if ok {
			break
		}

		low = high
		high = high * 2

		if high > build.BlockGasLimit {
			high = build.BlockGasLimit
			break
		}
	}

	checkThreshold := high / 100
	for (high - low) > checkThreshold {
		median := (low + high) / 2
		ok, err := canSucceed(median)
		if err != nil {
			return -1, xerrors.Errorf("searching for optimal gas limit failed: %w", err)
		}

		if ok {
			high = median
		} else {
			low = median
		}

		checkThreshold = median / 100
	}

	return high, nil
}

func traceContainsExitCode(et types.ExecutionTrace, ex exitcode.ExitCode) bool {
	if et.MsgRct.ExitCode == ex {
		return true
	}

	for _, et := range et.Subcalls {
		if traceContainsExitCode(et, ex) {
			return true
		}
	}

	return false
}

// ethGasSearch executes a message for gas estimation using the previously estimated gas.
// If the message fails due to an out of gas error then a gas search is performed.
// See gasSearch.
func ethGasSearch(
	ctx context.Context,
	cstore *store.ChainStore,
	smgr *stmgr.StateManager,
	mpool *messagepool.MessagePool,
	msgIn *types.Message,
	ts *types.TipSet,
) (int64, error) {
	msg := *msgIn
	currTs := ts

	res, priorMsgs, ts, err := gasEstimateCallWithGas(ctx, cstore, smgr, mpool, &msg, currTs)
	if err != nil {
		return -1, xerrors.Errorf("gas estimation failed: %w", err)
	}

	if res.MsgRct.ExitCode.IsSuccess() {
		return msg.GasLimit, nil
	}

	if traceContainsExitCode(res.ExecutionTrace, exitcode.SysErrOutOfGas) {
		ret, err := gasSearch(ctx, smgr, &msg, priorMsgs, ts)
		if err != nil {
			return -1, xerrors.Errorf("gas estimation search failed: %w", err)
		}

		ret = int64(float64(ret) * mpool.GetConfig().GasLimitOverestimation)
		return ret, nil
	}

	return -1, xerrors.Errorf("message execution failed: exit %s, reason: %s", res.MsgRct.ExitCode, res.Error)
}

func (a *EthModule) EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	msg, err := ethCallToFilecoinMessage(ctx, tx)
	if err != nil {
		return nil, xerrors.Errorf("failed to convert ethcall to filecoin message: %w", err)
	}

	ts, err := getTipsetByEthBlockNumberOrHash(ctx, a.Chain, blkParam)
	if err != nil {
		return nil, xerrors.Errorf("failed to process block param: %v; %w", blkParam, err)
	}

	invokeResult, err := a.applyMessage(ctx, msg, ts.Key())
	if err != nil {
		return nil, err
	}

	if msg.To == builtintypes.EthereumAddressManagerActorAddr {
		// As far as I can tell, the Eth API always returns empty on contract deployment
		return ethtypes.EthBytes{}, nil
	} else if len(invokeResult.MsgRct.Return) > 0 {
		return cbg.ReadByteArray(bytes.NewReader(invokeResult.MsgRct.Return), uint64(len(invokeResult.MsgRct.Return)))
	}

	return ethtypes.EthBytes{}, nil
}

func (e *EthEvent) EthGetLogs(ctx context.Context, filterSpec *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error) {
	if e.EventFilterManager == nil {
		return nil, api.ErrNotSupported
	}

	// Create a temporary filter
	f, err := e.installEthFilterSpec(ctx, filterSpec)
	if err != nil {
		return nil, err
	}
	ces := f.TakeCollectedEvents(ctx)

	_ = e.uninstallFilter(ctx, f)

	return ethFilterResultFromEvents(ces, e.SubManager.StateAPI)
}

func (e *EthEvent) EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	if e.FilterStore == nil {
		return nil, api.ErrNotSupported
	}

	f, err := e.FilterStore.Get(ctx, types.FilterID(id))
	if err != nil {
		return nil, err
	}

	switch fc := f.(type) {
	case filterEventCollector:
		return ethFilterResultFromEvents(fc.TakeCollectedEvents(ctx), e.SubManager.StateAPI)
	case filterTipSetCollector:
		return ethFilterResultFromTipSets(fc.TakeCollectedTipSets(ctx))
	case filterMessageCollector:
		return ethFilterResultFromMessages(fc.TakeCollectedMessages(ctx), e.SubManager.StateAPI)
	}

	return nil, xerrors.Errorf("unknown filter type")
}

func (e *EthEvent) EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	if e.FilterStore == nil {
		return nil, api.ErrNotSupported
	}

	f, err := e.FilterStore.Get(ctx, types.FilterID(id))
	if err != nil {
		return nil, err
	}

	switch fc := f.(type) {
	case filterEventCollector:
		return ethFilterResultFromEvents(fc.TakeCollectedEvents(ctx), e.SubManager.StateAPI)
	}

	return nil, xerrors.Errorf("wrong filter type")
}

func (e *EthEvent) installEthFilterSpec(ctx context.Context, filterSpec *ethtypes.EthFilterSpec) (*filter.EventFilter, error) {
	var (
		minHeight abi.ChainEpoch
		maxHeight abi.ChainEpoch
		tipsetCid cid.Cid
		addresses []address.Address
		keys      = map[string][][]byte{}
	)

	if filterSpec.BlockHash != nil {
		if filterSpec.FromBlock != nil || filterSpec.ToBlock != nil {
			return nil, xerrors.Errorf("must not specify block hash and from/to block")
		}

		tipsetCid = filterSpec.BlockHash.ToCid()
	} else {
		if filterSpec.FromBlock == nil || *filterSpec.FromBlock == "latest" {
			ts := e.Chain.GetHeaviestTipSet()
			minHeight = ts.Height()
		} else if *filterSpec.FromBlock == "earliest" {
			minHeight = 0
		} else if *filterSpec.FromBlock == "pending" {
			return nil, api.ErrNotSupported
		} else {
			if !strings.HasPrefix(*filterSpec.FromBlock, "0x") {
				return nil, xerrors.Errorf("FromBlock is not a hex")
			}
			epoch, err := ethtypes.EthUint64FromHex(*filterSpec.FromBlock)
			if err != nil {
				return nil, xerrors.Errorf("invalid epoch")
			}
			minHeight = abi.ChainEpoch(epoch)
		}

		if filterSpec.ToBlock == nil || *filterSpec.ToBlock == "latest" {
			// here latest means the latest at the time
			maxHeight = -1
		} else if *filterSpec.ToBlock == "earliest" {
			maxHeight = 0
		} else if *filterSpec.ToBlock == "pending" {
			return nil, api.ErrNotSupported
		} else {
			if !strings.HasPrefix(*filterSpec.ToBlock, "0x") {
				return nil, xerrors.Errorf("ToBlock is not a hex")
			}
			epoch, err := ethtypes.EthUint64FromHex(*filterSpec.ToBlock)
			if err != nil {
				return nil, xerrors.Errorf("invalid epoch")
			}
			maxHeight = abi.ChainEpoch(epoch)
		}

		// Validate height ranges are within limits set by node operator
		if minHeight == -1 && maxHeight > 0 {
			// Here the client is looking for events between the head and some future height
			ts := e.Chain.GetHeaviestTipSet()
			if maxHeight-ts.Height() > e.MaxFilterHeightRange {
				return nil, xerrors.Errorf("invalid epoch range: to block is too far in the future (maximum: %d)", e.MaxFilterHeightRange)
			}
		} else if minHeight >= 0 && maxHeight == -1 {
			// Here the client is looking for events between some time in the past and the current head
			ts := e.Chain.GetHeaviestTipSet()
			if ts.Height()-minHeight > e.MaxFilterHeightRange {
				return nil, xerrors.Errorf("invalid epoch range: from block is too far in the past (maximum: %d)", e.MaxFilterHeightRange)
			}

		} else if minHeight >= 0 && maxHeight >= 0 {
			if minHeight > maxHeight {
				return nil, xerrors.Errorf("invalid epoch range: to block (%d) must be after from block (%d)", minHeight, maxHeight)
			} else if maxHeight-minHeight > e.MaxFilterHeightRange {
				return nil, xerrors.Errorf("invalid epoch range: range between to and from blocks is too large (maximum: %d)", e.MaxFilterHeightRange)
			}
		}

	}

	// Convert all addresses to filecoin f4 addresses
	for _, ea := range filterSpec.Address {
		a, err := ea.ToFilecoinAddress()
		if err != nil {
			return nil, xerrors.Errorf("invalid address %x", ea)
		}
		addresses = append(addresses, a)
	}

	keys, err := parseEthTopics(filterSpec.Topics)
	if err != nil {
		return nil, err
	}

	return e.EventFilterManager.Install(ctx, minHeight, maxHeight, tipsetCid, addresses, keys)
}

func (e *EthEvent) EthNewFilter(ctx context.Context, filterSpec *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error) {
	if e.FilterStore == nil || e.EventFilterManager == nil {
		return ethtypes.EthFilterID{}, api.ErrNotSupported
	}

	f, err := e.installEthFilterSpec(ctx, filterSpec)
	if err != nil {
		return ethtypes.EthFilterID{}, err
	}

	if err := e.FilterStore.Add(ctx, f); err != nil {
		// Could not record in store, attempt to delete filter to clean up
		err2 := e.TipSetFilterManager.Remove(ctx, f.ID())
		if err2 != nil {
			return ethtypes.EthFilterID{}, xerrors.Errorf("encountered error %v while removing new filter due to %v", err2, err)
		}

		return ethtypes.EthFilterID{}, err
	}
	return ethtypes.EthFilterID(f.ID()), nil
}

func (e *EthEvent) EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	if e.FilterStore == nil || e.TipSetFilterManager == nil {
		return ethtypes.EthFilterID{}, api.ErrNotSupported
	}

	f, err := e.TipSetFilterManager.Install(ctx)
	if err != nil {
		return ethtypes.EthFilterID{}, err
	}

	if err := e.FilterStore.Add(ctx, f); err != nil {
		// Could not record in store, attempt to delete filter to clean up
		err2 := e.TipSetFilterManager.Remove(ctx, f.ID())
		if err2 != nil {
			return ethtypes.EthFilterID{}, xerrors.Errorf("encountered error %v while removing new filter due to %v", err2, err)
		}

		return ethtypes.EthFilterID{}, err
	}

	return ethtypes.EthFilterID(f.ID()), nil
}

func (e *EthEvent) EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	if e.FilterStore == nil || e.MemPoolFilterManager == nil {
		return ethtypes.EthFilterID{}, api.ErrNotSupported
	}

	f, err := e.MemPoolFilterManager.Install(ctx)
	if err != nil {
		return ethtypes.EthFilterID{}, err
	}

	if err := e.FilterStore.Add(ctx, f); err != nil {
		// Could not record in store, attempt to delete filter to clean up
		err2 := e.MemPoolFilterManager.Remove(ctx, f.ID())
		if err2 != nil {
			return ethtypes.EthFilterID{}, xerrors.Errorf("encountered error %v while removing new filter due to %v", err2, err)
		}

		return ethtypes.EthFilterID{}, err
	}

	return ethtypes.EthFilterID(f.ID()), nil
}

func (e *EthEvent) EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error) {
	if e.FilterStore == nil {
		return false, api.ErrNotSupported
	}

	f, err := e.FilterStore.Get(ctx, types.FilterID(id))
	if err != nil {
		if errors.Is(err, filter.ErrFilterNotFound) {
			return false, nil
		}
		return false, err
	}

	if err := e.uninstallFilter(ctx, f); err != nil {
		return false, err
	}

	return true, nil
}

func (e *EthEvent) uninstallFilter(ctx context.Context, f filter.Filter) error {
	switch f.(type) {
	case *filter.EventFilter:
		err := e.EventFilterManager.Remove(ctx, f.ID())
		if err != nil && !errors.Is(err, filter.ErrFilterNotFound) {
			return err
		}
	case *filter.TipSetFilter:
		err := e.TipSetFilterManager.Remove(ctx, f.ID())
		if err != nil && !errors.Is(err, filter.ErrFilterNotFound) {
			return err
		}
	case *filter.MemPoolFilter:
		err := e.MemPoolFilterManager.Remove(ctx, f.ID())
		if err != nil && !errors.Is(err, filter.ErrFilterNotFound) {
			return err
		}
	default:
		return xerrors.Errorf("unknown filter type")
	}

	return e.FilterStore.Remove(ctx, f.ID())
}

const (
	EthSubscribeEventTypeHeads               = "newHeads"
	EthSubscribeEventTypeLogs                = "logs"
	EthSubscribeEventTypePendingTransactions = "newPendingTransactions"
)

func (e *EthEvent) EthSubscribe(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error) {
	params, err := jsonrpc.DecodeParams[ethtypes.EthSubscribeParams](p)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("decoding params: %w", err)
	}

	if e.SubManager == nil {
		return ethtypes.EthSubscriptionID{}, api.ErrNotSupported
	}

	ethCb, ok := jsonrpc.ExtractReverseClient[api.EthSubscriberMethods](ctx)
	if !ok {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("connection doesn't support callbacks")
	}

	sub, err := e.SubManager.StartSubscription(e.SubscribtionCtx, ethCb.EthSubscription, e.uninstallFilter)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, err
	}

	switch params.EventType {
	case EthSubscribeEventTypeHeads:
		f, err := e.TipSetFilterManager.Install(ctx)
		if err != nil {
			// clean up any previous filters added and stop the sub
			_, _ = e.EthUnsubscribe(ctx, sub.id)
			return ethtypes.EthSubscriptionID{}, err
		}
		sub.addFilter(ctx, f)

	case EthSubscribeEventTypeLogs:
		keys := map[string][][]byte{}
		if params.Params != nil {
			var err error
			keys, err = parseEthTopics(params.Params.Topics)
			if err != nil {
				// clean up any previous filters added and stop the sub
				_, _ = e.EthUnsubscribe(ctx, sub.id)
				return ethtypes.EthSubscriptionID{}, err
			}
		}

		var addresses []address.Address
		if params.Params != nil {
			for _, ea := range params.Params.Address {
				a, err := ea.ToFilecoinAddress()
				if err != nil {
					return ethtypes.EthSubscriptionID{}, xerrors.Errorf("invalid address %x", ea)
				}
				addresses = append(addresses, a)
			}
		}

		f, err := e.EventFilterManager.Install(ctx, -1, -1, cid.Undef, addresses, keys)
		if err != nil {
			// clean up any previous filters added and stop the sub
			_, _ = e.EthUnsubscribe(ctx, sub.id)
			return ethtypes.EthSubscriptionID{}, err
		}
		sub.addFilter(ctx, f)
	case EthSubscribeEventTypePendingTransactions:
		f, err := e.MemPoolFilterManager.Install(ctx)
		if err != nil {
			// clean up any previous filters added and stop the sub
			_, _ = e.EthUnsubscribe(ctx, sub.id)
			return ethtypes.EthSubscriptionID{}, err
		}

		sub.addFilter(ctx, f)
	default:
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("unsupported event type: %s", params.EventType)
	}

	return sub.id, nil
}

func (e *EthEvent) EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error) {
	if e.SubManager == nil {
		return false, api.ErrNotSupported
	}

	err := e.SubManager.StopSubscription(ctx, id)
	if err != nil {
		return false, nil
	}

	return true, nil
}

// GC runs a garbage collection loop, deleting filters that have not been used within the ttl window
func (e *EthEvent) GC(ctx context.Context, ttl time.Duration) {
	if e.FilterStore == nil {
		return
	}

	tt := time.NewTicker(time.Minute * 30)
	defer tt.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tt.C:
			fs := e.FilterStore.NotTakenSince(time.Now().Add(-ttl))
			for _, f := range fs {
				if err := e.uninstallFilter(ctx, f); err != nil {
					log.Warnf("Failed to remove actor event filter during garbage collection: %v", err)
				}
			}
		}
	}
}

func calculateRewardsAndGasUsed(rewardPercentiles []float64, txGasRewards gasRewardSorter) ([]ethtypes.EthBigInt, int64) {
	var gasUsedTotal int64
	for _, tx := range txGasRewards {
		gasUsedTotal += tx.gasUsed
	}

	rewards := make([]ethtypes.EthBigInt, len(rewardPercentiles))
	for i := range rewards {
		rewards[i] = ethtypes.EthBigInt(types.NewInt(MinGasPremium))
	}

	if len(txGasRewards) == 0 {
		return rewards, gasUsedTotal
	}

	sort.Stable(txGasRewards)

	var idx int
	var sum int64
	for i, percentile := range rewardPercentiles {
		threshold := int64(float64(gasUsedTotal) * percentile / 100)
		for sum < threshold && idx < len(txGasRewards)-1 {
			sum += txGasRewards[idx].gasUsed
			idx++
		}
		rewards[i] = ethtypes.EthBigInt(txGasRewards[idx].premium)
	}

	return rewards, gasUsedTotal
}

type gasRewardTuple struct {
	gasUsed int64
	premium abi.TokenAmount
}

// sorted in ascending order
type gasRewardSorter []gasRewardTuple

func (g gasRewardSorter) Len() int { return len(g) }
func (g gasRewardSorter) Swap(i, j int) {
	g[i], g[j] = g[j], g[i]
}
func (g gasRewardSorter) Less(i, j int) bool {
	return g[i].premium.Int.Cmp(g[j].premium.Int) == -1
}
