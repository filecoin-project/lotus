package full

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"github.com/zyedidia/generic/queue"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"
	"github.com/filecoin-project/go-state-types/builtin/v10/evm"
	"github.com/filecoin-project/go-state-types/crypto"
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
	"github.com/filecoin-project/lotus/chain/vm"
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
	EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkOpt string) (ethtypes.EthUint64, error)
	EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error)
	EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*api.EthTxReceipt, error)
	EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkOpt string) (ethtypes.EthBytes, error)
	EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam string) (ethtypes.EthBytes, error)
	EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam string) (ethtypes.EthBigInt, error)
	EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error)
	EthChainId(ctx context.Context) (ethtypes.EthUint64, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error)
	EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error)
	EthEstimateGas(ctx context.Context, tx ethtypes.EthCall) (ethtypes.EthUint64, error)
	EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam string) (ethtypes.EthBytes, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error)
	EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error)
	Web3ClientVersion(ctx context.Context) (string, error)
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

func (a *EthModule) parseBlkParam(ctx context.Context, blkParam string, strict bool) (tipset *types.TipSet, err error) {
	if blkParam == "earliest" {
		return nil, fmt.Errorf("block param \"earliest\" is not supported")
	}

	head := a.Chain.GetHeaviestTipSet()
	switch blkParam {
	case "pending":
		return head, nil
	case "latest":
		parent, err := a.Chain.GetTipSetFromKey(ctx, head.Parents())
		if err != nil {
			return nil, fmt.Errorf("cannot get parent tipset")
		}
		return parent, nil
	default:
		var num ethtypes.EthUint64
		err := num.UnmarshalJSON([]byte(`"` + blkParam + `"`))
		if err != nil {
			return nil, fmt.Errorf("cannot parse block number: %v", err)
		}
		if abi.ChainEpoch(num) > head.Height()-1 {
			return nil, fmt.Errorf("requested a future epoch (beyond 'latest')")
		}
		ts, err := a.ChainAPI.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(num), head.Key())
		if err != nil {
			return nil, fmt.Errorf("cannot get tipset at height: %v", num)
		}
		if strict && ts.Height() != abi.ChainEpoch(num) {
			return nil, ErrNullRound
		}
		return ts, nil
	}
}

func (a *EthModule) EthGetBlockByNumber(ctx context.Context, blkParam string, fullTxInfo bool) (ethtypes.EthBlock, error) {
	ts, err := a.parseBlkParam(ctx, blkParam, true)
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
	hash, err := EthTxHashFromMessageCid(ctx, cid, a.StateAPI)
	if hash == ethtypes.EmptyEthHash {
		// not found
		return nil, nil
	}

	return &hash, err
}

func (a *EthModule) EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam string) (ethtypes.EthUint64, error) {
	addr, err := sender.ToFilecoinAddress()
	if err != nil {
		return ethtypes.EthUint64(0), nil
	}

	ts, err := a.parseBlkParam(ctx, blkParam, false)
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("cannot parse block param: %s", blkParam)
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
func (a *EthModule) EthGetCode(ctx context.Context, ethAddr ethtypes.EthAddress, blkParam string) (ethtypes.EthBytes, error) {
	to, err := ethAddr.ToFilecoinAddress()
	if err != nil {
		return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
	}

	ts, err := a.parseBlkParam(ctx, blkParam, false)
	if err != nil {
		return nil, xerrors.Errorf("cannot parse block param: %s", blkParam)
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

func (a *EthModule) EthGetStorageAt(ctx context.Context, ethAddr ethtypes.EthAddress, position ethtypes.EthBytes, blkParam string) (ethtypes.EthBytes, error) {
	ts, err := a.parseBlkParam(ctx, blkParam, false)
	if err != nil {
		return nil, xerrors.Errorf("cannot parse block param: %s", blkParam)
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

func (a *EthModule) EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam string) (ethtypes.EthBigInt, error) {
	filAddr, err := address.ToFilecoinAddress()
	if err != nil {
		return ethtypes.EthBigInt{}, err
	}

	ts, err := a.parseBlkParam(ctx, blkParam, false)
	if err != nil {
		return ethtypes.EthBigInt{}, xerrors.Errorf("cannot parse block param: %s", blkParam)
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

	ts, err := a.parseBlkParam(ctx, params.NewestBlkNum, false)
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

		// arrays should be reversed at the end
		baseFeeArray = append(baseFeeArray, ethtypes.EthBigInt(basefee))
		gasUsedRatioArray = append(gasUsedRatioArray, float64(totalGasUsed)/float64(build.BlockGasLimit))
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

func (a *EthModule) ethCallToFilecoinMessage(ctx context.Context, tx ethtypes.EthCall) (*types.Message, error) {
	var from address.Address
	if tx.From == nil || *tx.From == (ethtypes.EthAddress{}) {
		// Send from the filecoin "system" address.
		var err error
		from, err = (ethtypes.EthAddress{}).ToFilecoinAddress()
		if err != nil {
			return nil, fmt.Errorf("failed to construct the ethereum system address: %w", err)
		}
	} else {
		// The from address must be translatable to an f4 address.
		var err error
		from, err = tx.From.ToFilecoinAddress()
		if err != nil {
			return nil, fmt.Errorf("failed to translate sender address (%s): %w", tx.From.String(), err)
		}
		if p := from.Protocol(); p != address.Delegated {
			return nil, fmt.Errorf("expected a class 4 address, got: %d: %w", p, err)
		}
	}

	var params []byte
	if len(tx.Data) > 0 {
		initcode := abi.CborBytes(tx.Data)
		params2, err := actors.SerializeParams(&initcode)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize params: %w", err)
		}
		params = params2
	}

	var to address.Address
	var method abi.MethodNum
	if tx.To == nil {
		// this is a contract creation
		to = builtintypes.EthereumAddressManagerActorAddr
		method = builtintypes.MethodsEAM.CreateExternal
	} else {
		addr, err := tx.To.ToFilecoinAddress()
		if err != nil {
			return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
		}
		to = addr
		method = builtintypes.MethodsEVM.InvokeContract
	}

	return &types.Message{
		From:       from,
		To:         to,
		Value:      big.Int(tx.Value),
		Method:     method,
		Params:     params,
		GasLimit:   build.BlockGasLimit,
		GasFeeCap:  big.Zero(),
		GasPremium: big.Zero(),
	}, nil
}

func (a *EthModule) applyMessage(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (res *api.InvocResult, err error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("cannot get tipset: %w", err)
	}

	// Try calling until we find a height with no migration.
	for {
		res, err = a.StateManager.CallWithGas(ctx, msg, []types.ChainMsg{}, ts)
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
	msg, err := a.ethCallToFilecoinMessage(ctx, tx)
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

	canSucceed := func(limit int64) (bool, error) {
		msg.GasLimit = limit

		res, err := smgr.CallWithGas(ctx, &msg, priorMsgs, ts)
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

func (a *EthModule) EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam string) (ethtypes.EthBytes, error) {
	msg, err := a.ethCallToFilecoinMessage(ctx, tx)
	if err != nil {
		return nil, xerrors.Errorf("failed to convert ethcall to filecoin message: %w", err)
	}

	ts, err := a.parseBlkParam(ctx, blkParam, false)
	if err != nil {
		return nil, xerrors.Errorf("cannot parse block param: %s", blkParam)
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

		// TODO: derive a tipset hash from eth hash - might need to push this down into the EventFilterManager
	} else {
		if filterSpec.FromBlock == nil || *filterSpec.FromBlock == "latest" {
			ts := e.Chain.GetHeaviestTipSet()
			minHeight = ts.Height()
		} else if *filterSpec.FromBlock == "earliest" {
			minHeight = 0
		} else if *filterSpec.FromBlock == "pending" {
			return nil, api.ErrNotSupported
		} else {
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

type filterEventCollector interface {
	TakeCollectedEvents(context.Context) []*filter.CollectedEvent
}

type filterMessageCollector interface {
	TakeCollectedMessages(context.Context) []*types.SignedMessage
}

type filterTipSetCollector interface {
	TakeCollectedTipSets(context.Context) []types.TipSetKey
}

func ethLogFromEvent(entries []types.EventEntry) (data []byte, topics []ethtypes.EthHash, ok bool) {
	var (
		topicsFound      [4]bool
		topicsFoundCount int
		dataFound        bool
	)
	for _, entry := range entries {
		// Drop events with non-raw topics to avoid mistakes.
		if entry.Codec != cid.Raw {
			log.Warnw("did not expect an event entry with a non-raw codec", "codec", entry.Codec, "key", entry.Key)
			return nil, nil, false
		}
		// Check if the key is t1..t4
		if len(entry.Key) == 2 && "t1" <= entry.Key && entry.Key <= "t4" {
			// '1' - '1' == 0, etc.
			idx := int(entry.Key[1] - '1')

			// Drop events with mis-sized topics.
			if len(entry.Value) != 32 {
				log.Warnw("got an EVM event topic with an invalid size", "key", entry.Key, "size", len(entry.Value))
				return nil, nil, false
			}

			// Drop events with duplicate topics.
			if topicsFound[idx] {
				log.Warnw("got a duplicate EVM event topic", "key", entry.Key)
				return nil, nil, false
			}
			topicsFound[idx] = true
			topicsFoundCount++

			// Extend the topics array
			for len(topics) <= idx {
				topics = append(topics, ethtypes.EthHash{})
			}
			copy(topics[idx][:], entry.Value)
		} else if entry.Key == "d" {
			// Drop events with duplicate data fields.
			if dataFound {
				log.Warnw("got duplicate EVM event data")
				return nil, nil, false
			}

			dataFound = true
			data = entry.Value
		} else {
			// Skip entries we don't understand (makes it easier to extend things).
			// But we warn for now because we don't expect them.
			log.Warnw("unexpected event entry", "key", entry.Key)
		}

	}

	// Drop events with skipped topics.
	if len(topics) != topicsFoundCount {
		log.Warnw("EVM event topic length mismatch", "expected", len(topics), "actual", topicsFoundCount)
		return nil, nil, false
	}
	return data, topics, true
}

func ethFilterResultFromEvents(evs []*filter.CollectedEvent, sa StateAPI) (*ethtypes.EthFilterResult, error) {
	res := &ethtypes.EthFilterResult{}
	for _, ev := range evs {
		log := ethtypes.EthLog{
			Removed:          ev.Reverted,
			LogIndex:         ethtypes.EthUint64(ev.EventIdx),
			TransactionIndex: ethtypes.EthUint64(ev.MsgIdx),
			BlockNumber:      ethtypes.EthUint64(ev.Height),
		}
		var (
			err error
			ok  bool
		)

		log.Data, log.Topics, ok = ethLogFromEvent(ev.Entries)
		if !ok {
			continue
		}

		log.Address, err = ethtypes.EthAddressFromFilecoinAddress(ev.EmitterAddr)
		if err != nil {
			return nil, err
		}

		log.TransactionHash, err = EthTxHashFromMessageCid(context.TODO(), ev.MsgCid, sa)
		if err != nil {
			return nil, err
		}
		c, err := ev.TipSetKey.Cid()
		if err != nil {
			return nil, err
		}
		log.BlockHash, err = ethtypes.EthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, log)
	}

	return res, nil
}

func ethFilterResultFromTipSets(tsks []types.TipSetKey) (*ethtypes.EthFilterResult, error) {
	res := &ethtypes.EthFilterResult{}

	for _, tsk := range tsks {
		c, err := tsk.Cid()
		if err != nil {
			return nil, err
		}
		hash, err := ethtypes.EthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, hash)
	}

	return res, nil
}

func ethFilterResultFromMessages(cs []*types.SignedMessage, sa StateAPI) (*ethtypes.EthFilterResult, error) {
	res := &ethtypes.EthFilterResult{}

	for _, c := range cs {
		hash, err := EthTxHashFromSignedMessage(context.TODO(), c, sa)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, hash)
	}

	return res, nil
}

type EthSubscriptionManager struct {
	Chain    *store.ChainStore
	StateAPI StateAPI
	ChainAPI ChainAPI
	mu       sync.Mutex
	subs     map[ethtypes.EthSubscriptionID]*ethSubscription
}

func (e *EthSubscriptionManager) StartSubscription(ctx context.Context, out ethSubscriptionCallback, dropFilter func(context.Context, filter.Filter) error) (*ethSubscription, error) { // nolint
	rawid, err := uuid.NewRandom()
	if err != nil {
		return nil, xerrors.Errorf("new uuid: %w", err)
	}
	id := ethtypes.EthSubscriptionID{}
	copy(id[:], rawid[:]) // uuid is 16 bytes

	ctx, quit := context.WithCancel(ctx)

	sub := &ethSubscription{
		Chain:           e.Chain,
		StateAPI:        e.StateAPI,
		ChainAPI:        e.ChainAPI,
		uninstallFilter: dropFilter,
		id:              id,
		in:              make(chan interface{}, 200),
		out:             out,
		quit:            quit,

		toSend:   queue.New[[]byte](),
		sendCond: make(chan struct{}, 1),
	}

	e.mu.Lock()
	if e.subs == nil {
		e.subs = make(map[ethtypes.EthSubscriptionID]*ethSubscription)
	}
	e.subs[sub.id] = sub
	e.mu.Unlock()

	go sub.start(ctx)
	go sub.startOut(ctx)

	return sub, nil
}

func (e *EthSubscriptionManager) StopSubscription(ctx context.Context, id ethtypes.EthSubscriptionID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sub, ok := e.subs[id]
	if !ok {
		return xerrors.Errorf("subscription not found")
	}
	sub.stop()
	delete(e.subs, id)

	return nil
}

type ethSubscriptionCallback func(context.Context, jsonrpc.RawParams) error

const maxSendQueue = 20000

type ethSubscription struct {
	Chain           *store.ChainStore
	StateAPI        StateAPI
	ChainAPI        ChainAPI
	uninstallFilter func(context.Context, filter.Filter) error
	id              ethtypes.EthSubscriptionID
	in              chan interface{}
	out             ethSubscriptionCallback

	mu      sync.Mutex
	filters []filter.Filter
	quit    func()

	sendLk       sync.Mutex
	sendQueueLen int
	toSend       *queue.Queue[[]byte]
	sendCond     chan struct{}
}

func (e *ethSubscription) addFilter(ctx context.Context, f filter.Filter) {
	e.mu.Lock()
	defer e.mu.Unlock()

	f.SetSubChannel(e.in)
	e.filters = append(e.filters, f)
}

// sendOut processes the final subscription queue. It's here in case the subscriber
// is slow, and we need to buffer the messages.
func (e *ethSubscription) startOut(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.sendCond:
			e.sendLk.Lock()

			for !e.toSend.Empty() {
				front := e.toSend.Dequeue()
				e.sendQueueLen--

				e.sendLk.Unlock()

				if err := e.out(ctx, front); err != nil {
					log.Warnw("error sending subscription response, killing subscription", "sub", e.id, "error", err)
					e.stop()
					return
				}

				e.sendLk.Lock()
			}

			e.sendLk.Unlock()
		}
	}
}

func (e *ethSubscription) send(ctx context.Context, v interface{}) {
	resp := ethtypes.EthSubscriptionResponse{
		SubscriptionID: e.id,
		Result:         v,
	}

	outParam, err := json.Marshal(resp)
	if err != nil {
		log.Warnw("marshaling subscription response", "sub", e.id, "error", err)
		return
	}

	e.sendLk.Lock()
	defer e.sendLk.Unlock()

	e.toSend.Enqueue(outParam)

	e.sendQueueLen++
	if e.sendQueueLen > maxSendQueue {
		log.Warnw("subscription send queue full, killing subscription", "sub", e.id)
		e.stop()
		return
	}

	select {
	case e.sendCond <- struct{}{}:
	default: // already signalled, and we're holding the lock so we know that the event will be processed
	}
}

func (e *ethSubscription) start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case v := <-e.in:
			switch vt := v.(type) {
			case *filter.CollectedEvent:
				evs, err := ethFilterResultFromEvents([]*filter.CollectedEvent{vt}, e.StateAPI)
				if err != nil {
					continue
				}

				for _, r := range evs.Results {
					e.send(ctx, r)
				}
			case *types.TipSet:
				ev, err := newEthBlockFromFilecoinTipSet(ctx, vt, true, e.Chain, e.StateAPI)
				if err != nil {
					break
				}

				e.send(ctx, ev)
			case *types.SignedMessage: // mpool txid
				evs, err := ethFilterResultFromMessages([]*types.SignedMessage{vt}, e.StateAPI)
				if err != nil {
					continue
				}

				for _, r := range evs.Results {
					e.send(ctx, r)
				}
			default:
				log.Warnf("unexpected subscription value type: %T", vt)
			}
		}
	}
}

func (e *ethSubscription) stop() {
	e.mu.Lock()
	if e.quit == nil {
		e.mu.Unlock()
		return
	}

	if e.quit != nil {
		e.quit()
		e.quit = nil
		e.mu.Unlock()

		for _, f := range e.filters {
			// note: the context in actually unused in uninstallFilter
			if err := e.uninstallFilter(context.TODO(), f); err != nil {
				// this will leave the filter a zombie, collecting events up to the maximum allowed
				log.Warnf("failed to remove filter when unsubscribing: %v", err)
			}
		}
	}
}

func newEthBlockFromFilecoinTipSet(ctx context.Context, ts *types.TipSet, fullTxInfo bool, cs *store.ChainStore, sa StateAPI) (ethtypes.EthBlock, error) {
	parentKeyCid, err := ts.Parents().Cid()
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	parentBlkHash, err := ethtypes.EthHashFromCid(parentKeyCid)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}

	bn := ethtypes.EthUint64(ts.Height())

	blkCid, err := ts.Key().Cid()
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	blkHash, err := ethtypes.EthHashFromCid(blkCid)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}

	msgs, rcpts, err := messagesAndReceipts(ctx, ts, cs, sa)
	if err != nil {
		return ethtypes.EthBlock{}, xerrors.Errorf("failed to retrieve messages and receipts: %w", err)
	}

	block := ethtypes.NewEthBlock(len(msgs) > 0)

	gasUsed := int64(0)
	for i, msg := range msgs {
		rcpt := rcpts[i]
		ti := ethtypes.EthUint64(i)
		gasUsed += rcpt.GasUsed
		var smsg *types.SignedMessage
		switch msg := msg.(type) {
		case *types.SignedMessage:
			smsg = msg
		case *types.Message:
			smsg = &types.SignedMessage{
				Message: *msg,
				Signature: crypto.Signature{
					Type: crypto.SigTypeBLS,
				},
			}
		default:
			return ethtypes.EthBlock{}, xerrors.Errorf("failed to get signed msg %s: %w", msg.Cid(), err)
		}
		tx, err := newEthTxFromSignedMessage(ctx, smsg, sa)
		if err != nil {
			return ethtypes.EthBlock{}, xerrors.Errorf("failed to convert msg to ethTx: %w", err)
		}

		tx.ChainID = ethtypes.EthUint64(build.Eip155ChainId)
		tx.BlockHash = &blkHash
		tx.BlockNumber = &bn
		tx.TransactionIndex = &ti

		if fullTxInfo {
			block.Transactions = append(block.Transactions, tx)
		} else {
			block.Transactions = append(block.Transactions, tx.Hash.String())
		}
	}

	block.Hash = blkHash
	block.Number = bn
	block.ParentHash = parentBlkHash
	block.Timestamp = ethtypes.EthUint64(ts.Blocks()[0].Timestamp)
	block.BaseFeePerGas = ethtypes.EthBigInt{Int: ts.Blocks()[0].ParentBaseFee.Int}
	block.GasUsed = ethtypes.EthUint64(gasUsed)
	return block, nil
}

func messagesAndReceipts(ctx context.Context, ts *types.TipSet, cs *store.ChainStore, sa StateAPI) ([]types.ChainMsg, []types.MessageReceipt, error) {
	msgs, err := cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return nil, nil, xerrors.Errorf("error loading messages for tipset: %v: %w", ts, err)
	}

	_, rcptRoot, err := sa.StateManager.TipSetState(ctx, ts)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to compute state: %w", err)
	}

	rcpts, err := cs.ReadReceipts(ctx, rcptRoot)
	if err != nil {
		return nil, nil, xerrors.Errorf("error loading receipts for tipset: %v: %w", ts, err)
	}

	if len(msgs) != len(rcpts) {
		return nil, nil, xerrors.Errorf("receipts and message array lengths didn't match for tipset: %v: %w", ts, err)
	}

	return msgs, rcpts, nil
}

// lookupEthAddress makes its best effort at finding the Ethereum address for a
// Filecoin address. It does the following:
//
//  1. If the supplied address is an f410 address, we return its payload as the EthAddress.
//  2. Otherwise (f0, f1, f2, f3), we look up the actor on the state tree. If it has a delegated address, we return it if it's f410 address.
//  3. Otherwise, we fall back to returning a masked ID Ethereum address. If the supplied address is an f0 address, we
//     use that ID to form the masked ID address.
//  4. Otherwise, we fetch the actor's ID from the state tree and form the masked ID with it.
func lookupEthAddress(ctx context.Context, addr address.Address, sa StateAPI) (ethtypes.EthAddress, error) {
	// BLOCK A: We are trying to get an actual Ethereum address from an f410 address.
	// Attempt to convert directly, if it's an f4 address.
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(addr)
	if err == nil && !ethAddr.IsMaskedID() {
		return ethAddr, nil
	}

	// Lookup on the target actor and try to get an f410 address.
	if actor, err := sa.StateGetActor(ctx, addr, types.EmptyTSK); err != nil {
		return ethtypes.EthAddress{}, err
	} else if actor.Address != nil {
		if ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(*actor.Address); err == nil && !ethAddr.IsMaskedID() {
			return ethAddr, nil
		}
	}

	// BLOCK B: We gave up on getting an actual Ethereum address and are falling back to a Masked ID address.
	// Check if we already have an ID addr, and use it if possible.
	if err == nil && ethAddr.IsMaskedID() {
		return ethAddr, nil
	}

	// Otherwise, resolve the ID addr.
	idAddr, err := sa.StateLookupID(ctx, addr, types.EmptyTSK)
	if err != nil {
		return ethtypes.EthAddress{}, err
	}
	return ethtypes.EthAddressFromFilecoinAddress(idAddr)
}

func EthTxHashFromMessageCid(ctx context.Context, c cid.Cid, sa StateAPI) (ethtypes.EthHash, error) {
	smsg, err := sa.Chain.GetSignedMessage(ctx, c)
	if err == nil {
		// This is an Eth Tx, Secp message, Or BLS message in the mpool
		return EthTxHashFromSignedMessage(ctx, smsg, sa)
	}

	_, err = sa.Chain.GetMessage(ctx, c)
	if err == nil {
		// This is a BLS message
		return ethtypes.EthHashFromCid(c)
	}

	return ethtypes.EmptyEthHash, nil
}

func EthTxHashFromSignedMessage(ctx context.Context, smsg *types.SignedMessage, sa StateAPI) (ethtypes.EthHash, error) {
	if smsg.Signature.Type == crypto.SigTypeDelegated {
		ethTx, err := newEthTxFromSignedMessage(ctx, smsg, sa)
		if err != nil {
			return ethtypes.EmptyEthHash, err
		}
		return ethTx.Hash, nil
	} else if smsg.Signature.Type == crypto.SigTypeSecp256k1 {
		return ethtypes.EthHashFromCid(smsg.Cid())
	} else { // BLS message
		return ethtypes.EthHashFromCid(smsg.Message.Cid())
	}
}

func newEthTxFromSignedMessage(ctx context.Context, smsg *types.SignedMessage, sa StateAPI) (ethtypes.EthTx, error) {
	var tx ethtypes.EthTx
	var err error

	// This is an eth tx
	if smsg.Signature.Type == crypto.SigTypeDelegated {
		tx, err = ethtypes.EthTxFromSignedEthMessage(smsg)
		if err != nil {
			return ethtypes.EthTx{}, xerrors.Errorf("failed to convert from signed message: %w", err)
		}

		tx.Hash, err = tx.TxHash()
		if err != nil {
			return ethtypes.EthTx{}, xerrors.Errorf("failed to calculate hash for ethTx: %w", err)
		}

		fromAddr, err := lookupEthAddress(ctx, smsg.Message.From, sa)
		if err != nil {
			return ethtypes.EthTx{}, xerrors.Errorf("failed to resolve Ethereum address: %w", err)
		}

		tx.From = fromAddr
	} else if smsg.Signature.Type == crypto.SigTypeSecp256k1 { // Secp Filecoin Message
		tx = ethTxFromNativeMessage(ctx, smsg.VMMessage(), sa)
		tx.Hash, err = ethtypes.EthHashFromCid(smsg.Cid())
		if err != nil {
			return tx, err
		}
	} else { // BLS Filecoin message
		tx = ethTxFromNativeMessage(ctx, smsg.VMMessage(), sa)
		tx.Hash, err = ethtypes.EthHashFromCid(smsg.Message.Cid())
		if err != nil {
			return tx, err
		}
	}

	return tx, nil
}

// ethTxFromNativeMessage does NOT populate:
// - BlockHash
// - BlockNumber
// - TransactionIndex
// - Hash
func ethTxFromNativeMessage(ctx context.Context, msg *types.Message, sa StateAPI) ethtypes.EthTx {
	// We don't care if we error here, conversion is best effort for non-eth transactions
	from, _ := lookupEthAddress(ctx, msg.From, sa)
	to, _ := lookupEthAddress(ctx, msg.To, sa)
	return ethtypes.EthTx{
		To:                   &to,
		From:                 from,
		Nonce:                ethtypes.EthUint64(msg.Nonce),
		ChainID:              ethtypes.EthUint64(build.Eip155ChainId),
		Value:                ethtypes.EthBigInt(msg.Value),
		Type:                 ethtypes.Eip1559TxType,
		Gas:                  ethtypes.EthUint64(msg.GasLimit),
		MaxFeePerGas:         ethtypes.EthBigInt(msg.GasFeeCap),
		MaxPriorityFeePerGas: ethtypes.EthBigInt(msg.GasPremium),
		AccessList:           []ethtypes.EthHash{},
	}
}

// newEthTxFromMessageLookup creates an ethereum transaction from filecoin message lookup. If a negative txIdx is passed
// into the function, it looks up the transaction index of the message in the tipset, otherwise it uses the txIdx passed into the
// function
func newEthTxFromMessageLookup(ctx context.Context, msgLookup *api.MsgLookup, txIdx int, cs *store.ChainStore, sa StateAPI) (ethtypes.EthTx, error) {
	ts, err := cs.LoadTipSet(ctx, msgLookup.TipSet)
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	// This tx is located in the parent tipset
	parentTs, err := cs.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	parentTsCid, err := parentTs.Key().Cid()
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	// lookup the transactionIndex
	if txIdx < 0 {
		msgs, err := cs.MessagesForTipset(ctx, parentTs)
		if err != nil {
			return ethtypes.EthTx{}, err
		}
		for i, msg := range msgs {
			if msg.Cid() == msgLookup.Message {
				txIdx = i
				break
			}
		}
		if txIdx < 0 {
			return ethtypes.EthTx{}, fmt.Errorf("cannot find the msg in the tipset")
		}
	}

	blkHash, err := ethtypes.EthHashFromCid(parentTsCid)
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	smsg, err := getSignedMessage(ctx, cs, msgLookup.Message)
	if err != nil {
		return ethtypes.EthTx{}, xerrors.Errorf("failed to get signed msg: %w", err)
	}

	tx, err := newEthTxFromSignedMessage(ctx, smsg, sa)
	if err != nil {
		return ethtypes.EthTx{}, err
	}

	var (
		bn = ethtypes.EthUint64(parentTs.Height())
		ti = ethtypes.EthUint64(txIdx)
	)

	tx.ChainID = ethtypes.EthUint64(build.Eip155ChainId)
	tx.BlockHash = &blkHash
	tx.BlockNumber = &bn
	tx.TransactionIndex = &ti
	return tx, nil
}

func newEthTxReceipt(ctx context.Context, tx ethtypes.EthTx, lookup *api.MsgLookup, events []types.Event, cs *store.ChainStore, sa StateAPI) (api.EthTxReceipt, error) {
	var (
		transactionIndex ethtypes.EthUint64
		blockHash        ethtypes.EthHash
		blockNumber      ethtypes.EthUint64
	)

	if tx.TransactionIndex != nil {
		transactionIndex = *tx.TransactionIndex
	}
	if tx.BlockHash != nil {
		blockHash = *tx.BlockHash
	}
	if tx.BlockNumber != nil {
		blockNumber = *tx.BlockNumber
	}

	receipt := api.EthTxReceipt{
		TransactionHash:  tx.Hash,
		From:             tx.From,
		To:               tx.To,
		TransactionIndex: transactionIndex,
		BlockHash:        blockHash,
		BlockNumber:      blockNumber,
		Type:             ethtypes.EthUint64(2),
		Logs:             []ethtypes.EthLog{}, // empty log array is compulsory when no logs, or libraries like ethers.js break
		LogsBloom:        ethtypes.EmptyEthBloom[:],
	}

	if lookup.Receipt.ExitCode.IsSuccess() {
		receipt.Status = 1
	} else {
		receipt.Status = 0
	}

	receipt.GasUsed = ethtypes.EthUint64(lookup.Receipt.GasUsed)

	// TODO: handle CumulativeGasUsed
	receipt.CumulativeGasUsed = ethtypes.EmptyEthInt

	// TODO: avoid loading the tipset twice (once here, once when we convert the message to a txn)
	ts, err := cs.GetTipSetFromKey(ctx, lookup.TipSet)
	if err != nil {
		return api.EthTxReceipt{}, xerrors.Errorf("failed to lookup tipset %s when constructing the eth txn receipt: %w", lookup.TipSet, err)
	}

	baseFee := ts.Blocks()[0].ParentBaseFee
	gasOutputs := vm.ComputeGasOutputs(lookup.Receipt.GasUsed, int64(tx.Gas), baseFee, big.Int(tx.MaxFeePerGas), big.Int(tx.MaxPriorityFeePerGas), true)
	totalSpent := big.Sum(gasOutputs.BaseFeeBurn, gasOutputs.MinerTip, gasOutputs.OverEstimationBurn)

	effectiveGasPrice := big.Zero()
	if lookup.Receipt.GasUsed > 0 {
		effectiveGasPrice = big.Div(totalSpent, big.NewInt(lookup.Receipt.GasUsed))
	}
	receipt.EffectiveGasPrice = ethtypes.EthBigInt(effectiveGasPrice)

	if receipt.To == nil && lookup.Receipt.ExitCode.IsSuccess() {
		// Create and Create2 return the same things.
		var ret eam.CreateExternalReturn
		if err := ret.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)); err != nil {
			return api.EthTxReceipt{}, xerrors.Errorf("failed to parse contract creation result: %w", err)
		}
		addr := ethtypes.EthAddress(ret.EthAddress)
		receipt.ContractAddress = &addr
	}

	if len(events) > 0 {
		receipt.Logs = make([]ethtypes.EthLog, 0, len(events))
		for i, evt := range events {
			l := ethtypes.EthLog{
				Removed:          false,
				LogIndex:         ethtypes.EthUint64(i),
				TransactionHash:  tx.Hash,
				TransactionIndex: transactionIndex,
				BlockHash:        blockHash,
				BlockNumber:      blockNumber,
			}

			data, topics, ok := ethLogFromEvent(evt.Entries)
			if !ok {
				// not an eth event.
				continue
			}
			for _, topic := range topics {
				ethtypes.EthBloomSet(receipt.LogsBloom, topic[:])
			}
			l.Data = data
			l.Topics = topics

			addr, err := address.NewIDAddress(uint64(evt.Emitter))
			if err != nil {
				return api.EthTxReceipt{}, xerrors.Errorf("failed to create ID address: %w", err)
			}

			l.Address, err = lookupEthAddress(ctx, addr, sa)
			if err != nil {
				return api.EthTxReceipt{}, xerrors.Errorf("failed to resolve Ethereum address: %w", err)
			}

			ethtypes.EthBloomSet(receipt.LogsBloom, l.Address[:])
			receipt.Logs = append(receipt.Logs, l)
		}
	}

	return receipt, nil
}

func (m *EthTxHashManager) Apply(ctx context.Context, from, to *types.TipSet) error {
	for _, blk := range to.Blocks() {
		_, smsgs, err := m.StateAPI.Chain.MessagesForBlock(ctx, blk)
		if err != nil {
			return err
		}

		for _, smsg := range smsgs {
			if smsg.Signature.Type != crypto.SigTypeDelegated {
				continue
			}

			hash, err := EthTxHashFromSignedMessage(ctx, smsg, m.StateAPI)
			if err != nil {
				return err
			}

			err = m.TransactionHashLookup.UpsertHash(hash, smsg.Cid())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type EthTxHashManager struct {
	StateAPI              StateAPI
	TransactionHashLookup *ethhashlookup.EthTxHashLookup
}

func (m *EthTxHashManager) Revert(ctx context.Context, from, to *types.TipSet) error {
	return nil
}

func (m *EthTxHashManager) PopulateExistingMappings(ctx context.Context, minHeight abi.ChainEpoch) error {
	if minHeight < build.UpgradeHyggeHeight {
		minHeight = build.UpgradeHyggeHeight
	}

	ts := m.StateAPI.Chain.GetHeaviestTipSet()
	for ts.Height() > minHeight {
		for _, block := range ts.Blocks() {
			msgs, err := m.StateAPI.Chain.SecpkMessagesForBlock(ctx, block)
			if err != nil {
				// If we can't find the messages, we've either imported from snapshot or pruned the store
				log.Debug("exiting message mapping population at epoch ", ts.Height())
				return nil
			}

			for _, msg := range msgs {
				m.ProcessSignedMessage(ctx, msg)
			}
		}

		var err error
		ts, err = m.StateAPI.Chain.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *EthTxHashManager) ProcessSignedMessage(ctx context.Context, msg *types.SignedMessage) {
	if msg.Signature.Type != crypto.SigTypeDelegated {
		return
	}

	ethTx, err := newEthTxFromSignedMessage(ctx, msg, m.StateAPI)
	if err != nil {
		log.Errorf("error converting filecoin message to eth tx: %s", err)
		return
	}

	err = m.TransactionHashLookup.UpsertHash(ethTx.Hash, msg.Cid())
	if err != nil {
		log.Errorf("error inserting tx mapping to db: %s", err)
		return
	}
}

func WaitForMpoolUpdates(ctx context.Context, ch <-chan api.MpoolUpdate, manager *EthTxHashManager) {
	for {
		select {
		case <-ctx.Done():
			return
		case u := <-ch:
			if u.Type != api.MpoolAdd {
				continue
			}

			manager.ProcessSignedMessage(ctx, u.Message)
		}
	}
}

func EthTxHashGC(ctx context.Context, retentionDays int, manager *EthTxHashManager) {
	if retentionDays == 0 {
		return
	}

	gcPeriod := 1 * time.Hour
	for {
		entriesDeleted, err := manager.TransactionHashLookup.DeleteEntriesOlderThan(retentionDays)
		if err != nil {
			log.Errorf("error garbage collecting eth transaction hash database: %s", err)
		}
		log.Info("garbage collection run on eth transaction hash lookup database. %d entries deleted", entriesDeleted)
		time.Sleep(gcPeriod)
	}
}

func parseEthTopics(topics ethtypes.EthTopicSpec) (map[string][][]byte, error) {
	keys := map[string][][]byte{}
	for idx, vals := range topics {
		if len(vals) == 0 {
			continue
		}
		// Ethereum topics are emitted using `LOG{0..4}` opcodes resulting in topics1..4
		key := fmt.Sprintf("t%d", idx+1)
		for _, v := range vals {
			v := v // copy the ethhash to avoid repeatedly referencing the same one.
			keys[key] = append(keys[key], v[:])
		}
	}
	return keys, nil
}

const errorFunctionSelector = "\x08\xc3\x79\xa0" // Error(string)
const panicFunctionSelector = "\x4e\x48\x7b\x71" // Panic(uint256)
// Eth ABI (solidity) panic codes.
var panicErrorCodes map[uint64]string = map[uint64]string{
	0x00: "Panic()",
	0x01: "Assert()",
	0x11: "ArithmeticOverflow()",
	0x12: "DivideByZero()",
	0x21: "InvalidEnumVariant()",
	0x22: "InvalidStorageArray()",
	0x31: "PopEmptyArray()",
	0x32: "ArrayIndexOutOfBounds()",
	0x41: "OutOfMemory()",
	0x51: "CalledUninitializedFunction()",
}

// Parse an ABI encoded revert reason. This reason should be encoded as if it were the parameters to
// an `Error(string)` function call.
//
// See https://docs.soliditylang.org/en/latest/control-structures.html#panic-via-assert-and-error-via-require
func parseEthRevert(ret []byte) string {
	if len(ret) == 0 {
		return "none"
	}
	var cbytes abi.CborBytes
	if err := cbytes.UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
		return "ERROR: revert reason is not cbor encoded bytes"
	}
	if len(cbytes) == 0 {
		return "none"
	}
	// If it's not long enough to contain an ABI encoded response, return immediately.
	if len(cbytes) < 4+32 {
		return ethtypes.EthBytes(cbytes).String()
	}
	switch string(cbytes[:4]) {
	case panicFunctionSelector:
		cbytes := cbytes[4 : 4+32]
		// Read the and check the code.
		code, err := ethtypes.EthUint64FromBytes(cbytes)
		if err != nil {
			// If it's too big, just return the raw value.
			codeInt := big.PositiveFromUnsignedBytes(cbytes)
			return fmt.Sprintf("Panic(%s)", ethtypes.EthBigInt(codeInt).String())
		}
		if s, ok := panicErrorCodes[uint64(code)]; ok {
			return s
		}
		return fmt.Sprintf("Panic(0x%x)", code)
	case errorFunctionSelector:
		cbytes := cbytes[4:]
		cbytesLen := ethtypes.EthUint64(len(cbytes))
		// Read the and check the offset.
		offset, err := ethtypes.EthUint64FromBytes(cbytes[:32])
		if err != nil {
			break
		}
		if cbytesLen < offset {
			break
		}

		// Read and check the length.
		if cbytesLen-offset < 32 {
			break
		}
		start := offset + 32
		length, err := ethtypes.EthUint64FromBytes(cbytes[offset : offset+32])
		if err != nil {
			break
		}
		if cbytesLen-start < length {
			break
		}
		// Slice the error message.
		return fmt.Sprintf("Error(%s)", cbytes[start:start+length])
	}
	return ethtypes.EthBytes(cbytes).String()
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

func getSignedMessage(ctx context.Context, cs *store.ChainStore, msgCid cid.Cid) (*types.SignedMessage, error) {
	smsg, err := cs.GetSignedMessage(ctx, msgCid)
	if err != nil {
		// We couldn't find the signed message, it might be a BLS message, so search for a regular message.
		msg, err := cs.GetMessage(ctx, msgCid)
		if err != nil {
			return nil, xerrors.Errorf("failed to find msg %s: %w", msgCid, err)
		}
		smsg = &types.SignedMessage{
			Message: *msg,
			Signature: crypto.Signature{
				Type: crypto.SigTypeBLS,
			},
		}
	}

	return smsg, nil
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
