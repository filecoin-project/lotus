package full

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"
	"github.com/filecoin-project/go-state-types/builtin/v10/evm"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/ethhashlookup"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type EthModuleAPI interface {
	EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error)
	EthAccounts(ctx context.Context) ([]ethtypes.EthAddress, error)
	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum ethtypes.EthUint64) (ethtypes.EthUint64, error)
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error)
	EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error)
	EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error)
	EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error)
	EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error)
	EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkOpt string) (ethtypes.EthUint64, error)
	EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (ethtypes.EthTx, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum ethtypes.EthUint64, txIndex ethtypes.EthUint64) (ethtypes.EthTx, error)
	EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkOpt string) (ethtypes.EthBytes, error)
	EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam string) (ethtypes.EthBytes, error)
	EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam string) (ethtypes.EthBigInt, error)
	EthFeeHistory(ctx context.Context, blkCount ethtypes.EthUint64, newestBlk string, rewardPercentiles []float64) (ethtypes.EthFeeHistory, error)
	EthChainId(ctx context.Context) (ethtypes.EthUint64, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error)
	EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error)
	EthEstimateGas(ctx context.Context, tx ethtypes.EthCall) (ethtypes.EthUint64, error)
	EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam string) (ethtypes.EthBytes, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error)
	EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error)
}

type EthEventAPI interface {
	EthGetLogs(ctx context.Context, filter *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error)
	EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error)
	EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error)
	EthNewFilter(ctx context.Context, filter *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error)
	EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error)
	EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error)
	EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error)
	EthSubscribe(ctx context.Context, eventType string, params *ethtypes.EthSubscriptionParams) (<-chan ethtypes.EthSubscriptionResponse, error)
	EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error)
}

var (
	_ EthModuleAPI = *new(api.FullNode)
	_ EthEventAPI  = *new(api.FullNode)
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
}

var _ EthEventAPI = (*EthEvent)(nil)

type EthAPI struct {
	fx.In

	Chain *store.ChainStore

	EthModuleAPI
	EthEventAPI
}

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
	return newEthBlockFromFilecoinTipSet(ctx, ts, fullTxInfo, a.Chain, a.ChainAPI, a.StateAPI)
}

func (a *EthModule) parseBlkParam(ctx context.Context, blkParam string) (tipset *types.TipSet, err error) {
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
		ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(num), nil, false)
		if err != nil {
			return nil, fmt.Errorf("cannot get tipset at height: %v", num)
		}
		return ts, nil
	}
}

func (a *EthModule) EthGetBlockByNumber(ctx context.Context, blkParam string, fullTxInfo bool) (ethtypes.EthBlock, error) {
	ts, err := a.parseBlkParam(ctx, blkParam)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	return newEthBlockFromFilecoinTipSet(ctx, ts, fullTxInfo, a.Chain, a.ChainAPI, a.StateAPI)
}

func (a *EthModule) EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error) {
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
	msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, c, api.LookbackNoLimit, true)
	if err == nil {
		tx, err := newEthTxFromFilecoinMessageLookup(ctx, msgLookup, -1, a.Chain, a.StateAPI)
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
			tx, err := NewEthTxFromFilecoinMessage(ctx, p, a.StateAPI)
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

	_, err = a.StateAPI.Chain.GetSignedMessage(ctx, c)
	if err == nil {
		// This is an Eth Tx, Secp message, Or BLS message in the mpool
		return &c, nil
	}

	_, err = a.StateAPI.Chain.GetMessage(ctx, c)
	if err == nil {
		// This is a BLS message
		return &c, nil
	}

	// Ethereum clients expect an empty response when the message was not found
	return nil, nil
}

func (a *EthModule) EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error) {
	hash, err := EthTxHashFromFilecoinMessageCid(ctx, cid, a.StateAPI)
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

	ts, err := a.parseBlkParam(ctx, blkParam)
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("cannot parse block param: %s", blkParam)
	}

	nonce, err := a.Mpool.GetNonce(ctx, addr, ts.Key())
	if err != nil {
		return ethtypes.EthUint64(0), nil
	}
	return ethtypes.EthUint64(nonce), nil
}

func (a *EthModule) EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error) {
	c, err := a.EthTxHashManager.TransactionHashLookup.GetCidFromHash(txHash)
	if err != nil {
		log.Debug("could not find transaction hash %s in lookup table", txHash.String())
	}

	// This isn't an eth transaction we have the mapping for, so let's look it up as a filecoin message
	if c == cid.Undef {
		c = txHash.ToCid()
	}

	msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, c, api.LookbackNoLimit, true)
	if err != nil || msgLookup == nil {
		return nil, nil
	}

	tx, err := newEthTxFromFilecoinMessageLookup(ctx, msgLookup, -1, a.Chain, a.StateAPI)
	if err != nil {
		return nil, nil
	}

	replay, err := a.StateAPI.StateReplay(ctx, types.EmptyTSK, c)
	if err != nil {
		return nil, nil
	}

	var events []types.Event
	if rct := replay.MsgRct; rct != nil && rct.EventsRoot != nil {
		events, err = a.ChainAPI.ChainGetEvents(ctx, *rct.EventsRoot)
		if err != nil {
			return nil, nil
		}
	}

	receipt, err := newEthTxReceipt(ctx, tx, msgLookup, replay, events, a.StateAPI)
	if err != nil {
		return nil, nil
	}

	return &receipt, nil
}

func (a *EthModule) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (ethtypes.EthTx, error) {
	return ethtypes.EthTx{}, nil
}

func (a *EthModule) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum ethtypes.EthUint64, txIndex ethtypes.EthUint64) (ethtypes.EthTx, error) {
	return ethtypes.EthTx{}, nil
}

// EthGetCode returns string value of the compiled bytecode
func (a *EthModule) EthGetCode(ctx context.Context, ethAddr ethtypes.EthAddress, blkParam string) (ethtypes.EthBytes, error) {
	to, err := ethAddr.ToFilecoinAddress()
	if err != nil {
		return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
	}

	// use the system actor as the caller
	from, err := address.NewIDAddress(0)
	if err != nil {
		return nil, fmt.Errorf("failed to construct system sender address: %w", err)
	}
	msg := &types.Message{
		From:       from,
		To:         to,
		Value:      big.Zero(),
		Method:     builtintypes.MethodsEVM.GetBytecode,
		Params:     nil,
		GasLimit:   build.BlockGasLimit,
		GasFeeCap:  big.Zero(),
		GasPremium: big.Zero(),
	}

	ts, err := a.parseBlkParam(ctx, blkParam)
	if err != nil {
		return nil, xerrors.Errorf("cannot parse block param: %s", blkParam)
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
		// if the call resulted in error, this is not an EVM smart contract;
		// return no bytecode.
		return nil, nil
	}

	if res.MsgRct == nil {
		return nil, fmt.Errorf("no message receipt")
	}

	if res.MsgRct.ExitCode.IsError() {
		return nil, xerrors.Errorf("message execution failed: exit %s, reason: %s", res.MsgRct.ExitCode, res.Error)
	}

	var bytecodeCid cbg.CborCid
	if err := bytecodeCid.UnmarshalCBOR(bytes.NewReader(res.MsgRct.Return)); err != nil {
		return nil, fmt.Errorf("failed to decode EVM bytecode CID: %w", err)
	}

	blk, err := a.Chain.StateBlockstore().Get(ctx, cid.Cid(bytecodeCid))
	if err != nil {
		return nil, fmt.Errorf("failed to get EVM bytecode: %w", err)
	}

	return blk.RawData(), nil
}

func (a *EthModule) EthGetStorageAt(ctx context.Context, ethAddr ethtypes.EthAddress, position ethtypes.EthBytes, blkParam string) (ethtypes.EthBytes, error) {
	l := len(position)
	if l > 32 {
		return nil, fmt.Errorf("supplied storage key is too long")
	}

	// pad with zero bytes if smaller than 32 bytes
	position = append(make([]byte, 32-l, 32-l), position...)

	to, err := ethAddr.ToFilecoinAddress()
	if err != nil {
		return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
	}

	// use the system actor as the caller
	from, err := address.NewIDAddress(0)
	if err != nil {
		return nil, fmt.Errorf("failed to construct system sender address: %w", err)
	}

	// TODO super duper hack (raulk). The EVM runtime actor uses the U256 parameter type in
	//  GetStorageAtParams, which serializes as a hex-encoded string. It should serialize
	//  as bytes. We didn't get to fix in time for Iron, so for now we just pass
	//  through the hex-encoded value passed through the Eth JSON-RPC API, by remarshalling it.
	//  We don't fix this at origin (builtin-actors) because we are not updating the bundle
	//  for Iron.
	tmp, err := position.MarshalJSON()
	if err != nil {
		panic(err)
	}
	params, err := actors.SerializeParams(&evm.GetStorageAtParams{
		StorageKey: tmp[1 : len(tmp)-1], // TODO strip the JSON-encoding quotes -- yuck
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

	ts := a.Chain.GetHeaviestTipSet()

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
		return nil, fmt.Errorf("no message receipt")
	}

	return res.MsgRct.Return, nil
}

func (a *EthModule) EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam string) (ethtypes.EthBigInt, error) {
	filAddr, err := address.ToFilecoinAddress()
	if err != nil {
		return ethtypes.EthBigInt{}, err
	}

	ts, err := a.parseBlkParam(ctx, blkParam)
	if err != nil {
		return ethtypes.EthBigInt{}, xerrors.Errorf("cannot parse block param: %s", blkParam)
	}

	actor, err := a.StateGetActor(ctx, filAddr, ts.Key())
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

func (a *EthModule) EthFeeHistory(ctx context.Context, blkCount ethtypes.EthUint64, newestBlkNum string, rewardPercentiles []float64) (ethtypes.EthFeeHistory, error) {
	if blkCount > 1024 {
		return ethtypes.EthFeeHistory{}, fmt.Errorf("block count should be smaller than 1024")
	}

	newestBlkHeight := uint64(a.Chain.GetHeaviestTipSet().Height())

	// TODO https://github.com/filecoin-project/ref-fvm/issues/1016
	var blkNum ethtypes.EthUint64
	err := blkNum.UnmarshalJSON([]byte(`"` + newestBlkNum + `"`))
	if err == nil && uint64(blkNum) < newestBlkHeight {
		newestBlkHeight = uint64(blkNum)
	}

	// Deal with the case that the chain is shorter than the number of
	// requested blocks.
	oldestBlkHeight := uint64(1)
	if uint64(blkCount) <= newestBlkHeight {
		oldestBlkHeight = newestBlkHeight - uint64(blkCount) + 1
	}

	ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(newestBlkHeight), nil, false)
	if err != nil {
		return ethtypes.EthFeeHistory{}, fmt.Errorf("cannot load find block height: %v", newestBlkHeight)
	}

	// FIXME: baseFeePerGas should include the next block after the newest of the returned range, because this
	// can be inferred from the newest block. we use the newest block's baseFeePerGas for now but need to fix it
	// In other words, due to deferred execution, we might not be returning the most useful value here for the client.
	baseFeeArray := []ethtypes.EthBigInt{ethtypes.EthBigInt(ts.Blocks()[0].ParentBaseFee)}
	gasUsedRatioArray := []float64{}

	for ts.Height() >= abi.ChainEpoch(oldestBlkHeight) {
		// Unfortunately we need to rebuild the full message view so we can
		// totalize gas used in the tipset.
		block, err := newEthBlockFromFilecoinTipSet(ctx, ts, false, a.Chain, a.ChainAPI, a.StateAPI)
		if err != nil {
			return ethtypes.EthFeeHistory{}, fmt.Errorf("cannot create eth block: %v", err)
		}

		// both arrays should be reversed at the end
		baseFeeArray = append(baseFeeArray, ethtypes.EthBigInt(ts.Blocks()[0].ParentBaseFee))
		gasUsedRatioArray = append(gasUsedRatioArray, float64(block.GasUsed)/float64(build.BlockGasLimit))

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

	return ethtypes.EthFeeHistory{
		OldestBlock:   oldestBlkHeight,
		BaseFeePerGas: baseFeeArray,
		GasUsedRatio:  gasUsedRatioArray,
	}, nil
}

func (a *EthModule) NetVersion(ctx context.Context) (string, error) {
	// Note that networkId is not encoded in hex
	nv, err := a.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(uint64(nv), 10), nil
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

	_, err = a.StateAPI.StateGetActor(ctx, smsg.Message.To, types.EmptyTSK)
	if err != nil {
		// if actor does not exist on chain yet, set the method to 0 because
		// placeholders only implement method 0
		smsg.Message.Method = builtinactors.MethodSend
	}

	_, err = a.MpoolAPI.MpoolPush(ctx, smsg)
	if err != nil {
		return ethtypes.EmptyEthHash, err
	}

	return ethtypes.EthHashFromTxBytes(rawTx), nil
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
	var to address.Address
	var method abi.MethodNum
	if tx.To == nil {
		// this is a contract creation
		to = builtintypes.EthereumAddressManagerActorAddr

		initcode := abi.CborBytes(tx.Data)
		params2, err := actors.SerializeParams(&initcode)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize Create params: %w", err)
		}
		params = params2
		method = builtintypes.MethodsEAM.CreateExternal
	} else {
		addr, err := tx.To.ToFilecoinAddress()
		if err != nil {
			return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
		}
		to = addr

		if len(tx.Data) > 0 {
			var buf bytes.Buffer
			if err := cbg.WriteByteArray(&buf, tx.Data); err != nil {
				return nil, fmt.Errorf("failed to encode tx input into a cbor byte-string")
			}
			params = buf.Bytes()
			method = builtintypes.MethodsEVM.InvokeContract
		} else {
			method = builtintypes.MethodSend
		}
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
		return nil, xerrors.Errorf("message execution failed: exit %s, msg receipt: %s, reason: %s", res.MsgRct.ExitCode, res.MsgRct.Return, res.Error)
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

	msg, err = a.GasAPI.GasEstimateMessageGas(ctx, msg, nil, types.EmptyTSK)
	if err != nil {
		return ethtypes.EthUint64(0), err
	}

	return ethtypes.EthUint64(msg.GasLimit), nil
}

func (a *EthModule) EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam string) (ethtypes.EthBytes, error) {
	msg, err := a.ethCallToFilecoinMessage(ctx, tx)
	if err != nil {
		return nil, err
	}
	ts, err := a.parseBlkParam(ctx, blkParam)
	if err != nil {
		return nil, xerrors.Errorf("cannot parse block param: %s", blkParam)
	}

	invokeResult, err := a.applyMessage(ctx, msg, ts.Key())
	if err != nil {
		return nil, err
	}
	if len(invokeResult.MsgRct.Return) > 0 {
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

	for idx, vals := range filterSpec.Topics {
		if len(vals) == 0 {
			continue
		}
		// Ethereum topics are emitted using `LOG{0..4}` opcodes resulting in topics1..4
		key := fmt.Sprintf("topic%d", idx+1)
		for _, v := range vals {
			buf := make([]byte, len(v[:]))
			copy(buf, v[:])
			keys[key] = append(keys[key], buf)
		}
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
	EthSubscribeEventTypeHeads = "newHeads"
	EthSubscribeEventTypeLogs  = "logs"
)

func (e *EthEvent) EthSubscribe(ctx context.Context, eventType string, params *ethtypes.EthSubscriptionParams) (<-chan ethtypes.EthSubscriptionResponse, error) {
	if e.SubManager == nil {
		return nil, api.ErrNotSupported
	}
	// Note that go-jsonrpc will set the method field of the response to "xrpc.ch.val" but the ethereum api expects the name of the
	// method to be "eth_subscription". This probably doesn't matter in practice.

	sub, err := e.SubManager.StartSubscription(ctx)
	if err != nil {
		return nil, err
	}

	switch eventType {
	case EthSubscribeEventTypeHeads:
		f, err := e.TipSetFilterManager.Install(ctx)
		if err != nil {
			// clean up any previous filters added and stop the sub
			_, _ = e.EthUnsubscribe(ctx, sub.id)
			return nil, err
		}
		sub.addFilter(ctx, f)

	case EthSubscribeEventTypeLogs:
		keys := map[string][][]byte{}
		if params != nil {
			for idx, vals := range params.Topics {
				// Ethereum topics are emitted using `LOG{0..4}` opcodes resulting in topics1..4
				key := fmt.Sprintf("topic%d", idx+1)
				keyvals := make([][]byte, len(vals))
				for i, v := range vals {
					keyvals[i] = v[:]
				}
				keys[key] = keyvals
			}
		}

		f, err := e.EventFilterManager.Install(ctx, -1, -1, cid.Undef, []address.Address{}, keys)
		if err != nil {
			// clean up any previous filters added and stop the sub
			_, _ = e.EthUnsubscribe(ctx, sub.id)
			return nil, err
		}
		sub.addFilter(ctx, f)
	default:
		return nil, xerrors.Errorf("unsupported event type: %s", eventType)
	}

	return sub.out, nil
}

func (e *EthEvent) EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error) {
	if e.SubManager == nil {
		return false, api.ErrNotSupported
	}

	filters, err := e.SubManager.StopSubscription(ctx, id)
	if err != nil {
		return false, nil
	}

	for _, f := range filters {
		if err := e.uninstallFilter(ctx, f); err != nil {
			// this will leave the filter a zombie, collecting events up to the maximum allowed
			log.Warnf("failed to remove filter when unsubscribing: %v", err)
		}
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

func ethFilterResultFromEvents(evs []*filter.CollectedEvent, sa StateAPI) (*ethtypes.EthFilterResult, error) {
	res := &ethtypes.EthFilterResult{}
	for _, ev := range evs {
		log := ethtypes.EthLog{
			Removed:          ev.Reverted,
			LogIndex:         ethtypes.EthUint64(ev.EventIdx),
			TransactionIndex: ethtypes.EthUint64(ev.MsgIdx),
			BlockNumber:      ethtypes.EthUint64(ev.Height),
		}

		var err error

		for _, entry := range ev.Entries {
			value := ethtypes.EthBytes(leftpad32(entry.Value)) // value has already been cbor-decoded but see https://github.com/filecoin-project/ref-fvm/issues/1345
			if entry.Key == ethtypes.EthTopic1 || entry.Key == ethtypes.EthTopic2 || entry.Key == ethtypes.EthTopic3 || entry.Key == ethtypes.EthTopic4 {
				log.Topics = append(log.Topics, value)
			} else {
				log.Data = value
			}
		}

		log.Address, err = ethtypes.EthAddressFromFilecoinAddress(ev.EmitterAddr)
		if err != nil {
			return nil, err
		}

		log.TransactionHash, err = EthTxHashFromFilecoinMessageCid(context.TODO(), ev.MsgCid, sa)
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
		hash, err := EthTxHashFromSignedFilecoinMessage(context.TODO(), c, sa)
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

func (e *EthSubscriptionManager) StartSubscription(ctx context.Context) (*ethSubscription, error) { // nolint
	rawid, err := uuid.NewRandom()
	if err != nil {
		return nil, xerrors.Errorf("new uuid: %w", err)
	}
	id := ethtypes.EthSubscriptionID{}
	copy(id[:], rawid[:]) // uuid is 16 bytes

	ctx, quit := context.WithCancel(ctx)

	sub := &ethSubscription{
		Chain:    e.Chain,
		StateAPI: e.StateAPI,
		ChainAPI: e.ChainAPI,
		id:       id,
		in:       make(chan interface{}, 200),
		out:      make(chan ethtypes.EthSubscriptionResponse, 20),
		quit:     quit,
	}

	e.mu.Lock()
	if e.subs == nil {
		e.subs = make(map[ethtypes.EthSubscriptionID]*ethSubscription)
	}
	e.subs[sub.id] = sub
	e.mu.Unlock()

	go sub.start(ctx)

	return sub, nil
}

func (e *EthSubscriptionManager) StopSubscription(ctx context.Context, id ethtypes.EthSubscriptionID) ([]filter.Filter, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	sub, ok := e.subs[id]
	if !ok {
		return nil, xerrors.Errorf("subscription not found")
	}
	sub.stop()
	delete(e.subs, id)

	return sub.filters, nil
}

type ethSubscription struct {
	Chain    *store.ChainStore
	StateAPI StateAPI
	ChainAPI ChainAPI
	id       ethtypes.EthSubscriptionID
	in       chan interface{}
	out      chan ethtypes.EthSubscriptionResponse

	mu      sync.Mutex
	filters []filter.Filter
	quit    func()
}

func (e *ethSubscription) addFilter(ctx context.Context, f filter.Filter) {
	e.mu.Lock()
	defer e.mu.Unlock()

	f.SetSubChannel(e.in)
	e.filters = append(e.filters, f)
}

func (e *ethSubscription) start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case v := <-e.in:
			resp := ethtypes.EthSubscriptionResponse{
				SubscriptionID: e.id,
			}

			var err error
			switch vt := v.(type) {
			case *filter.CollectedEvent:
				resp.Result, err = ethFilterResultFromEvents([]*filter.CollectedEvent{vt}, e.StateAPI)
			case *types.TipSet:
				eb, err := newEthBlockFromFilecoinTipSet(ctx, vt, true, e.Chain, e.ChainAPI, e.StateAPI)
				if err != nil {
					break
				}

				resp.Result = eb
			default:
				log.Warnf("unexpected subscription value type: %T", vt)
			}

			if err != nil {
				continue
			}

			select {
			case e.out <- resp:
			default:
				// Skip if client is not reading responses
			}
		}
	}
}

func (e *ethSubscription) stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.quit != nil {
		e.quit()
		close(e.out)
		e.quit = nil
	}
}

func newEthBlockFromFilecoinTipSet(ctx context.Context, ts *types.TipSet, fullTxInfo bool, cs *store.ChainStore, ca ChainAPI, sa StateAPI) (ethtypes.EthBlock, error) {
	parent, err := cs.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	parentKeyCid, err := parent.Key().Cid()
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	parentBlkHash, err := ethtypes.EthHashFromCid(parentKeyCid)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}

	blkCid, err := ts.Key().Cid()
	if err != nil {
		return ethtypes.EthBlock{}, err
	}
	blkHash, err := ethtypes.EthHashFromCid(blkCid)
	if err != nil {
		return ethtypes.EthBlock{}, err
	}

	msgs, err := cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return ethtypes.EthBlock{}, xerrors.Errorf("error loading messages for tipset: %v: %w", ts, err)
	}

	block := ethtypes.NewEthBlock()

	// this seems to be a very expensive way to get gasUsed of the block. may need to find an efficient way to do it
	gasUsed := int64(0)
	for txIdx, msg := range msgs {
		msgLookup, err := sa.StateSearchMsg(ctx, types.EmptyTSK, msg.Cid(), api.LookbackNoLimit, false)
		if err != nil || msgLookup == nil {
			return ethtypes.EthBlock{}, nil
		}
		gasUsed += msgLookup.Receipt.GasUsed

		tx, err := newEthTxFromFilecoinMessageLookup(ctx, msgLookup, txIdx, cs, sa)
		if err != nil {
			return ethtypes.EthBlock{}, nil
		}

		if fullTxInfo {
			block.Transactions = append(block.Transactions, tx)
		} else {
			block.Transactions = append(block.Transactions, tx.Hash.String())
		}
	}

	block.Hash = blkHash
	block.Number = ethtypes.EthUint64(ts.Height())
	block.ParentHash = parentBlkHash
	block.Timestamp = ethtypes.EthUint64(ts.Blocks()[0].Timestamp)
	block.BaseFeePerGas = ethtypes.EthBigInt{Int: ts.Blocks()[0].ParentBaseFee.Int}
	block.GasUsed = ethtypes.EthUint64(gasUsed)
	return block, nil
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

func EthTxHashFromFilecoinMessageCid(ctx context.Context, c cid.Cid, sa StateAPI) (ethtypes.EthHash, error) {
	smsg, err := sa.Chain.GetSignedMessage(ctx, c)
	if err == nil {
		// This is an Eth Tx, Secp message, Or BLS message in the mpool
		return EthTxHashFromSignedFilecoinMessage(ctx, smsg, sa)
	}

	_, err = sa.Chain.GetMessage(ctx, c)
	if err == nil {
		// This is a BLS message
		return ethtypes.EthHashFromCid(c)
	}

	return ethtypes.EmptyEthHash, nil
}

func EthTxHashFromSignedFilecoinMessage(ctx context.Context, smsg *types.SignedMessage, sa StateAPI) (ethtypes.EthHash, error) {
	if smsg.Signature.Type == crypto.SigTypeDelegated {
		ethTx, err := NewEthTxFromFilecoinMessage(ctx, smsg, sa)
		if err != nil {
			return ethtypes.EmptyEthHash, err
		}
		return ethTx.Hash, nil
	}

	return ethtypes.EthHashFromCid(smsg.Cid())
}

func NewEthTxFromFilecoinMessage(ctx context.Context, smsg *types.SignedMessage, sa StateAPI) (ethtypes.EthTx, error) {
	// Ignore errors here so we can still parse non-eth messages
	fromEthAddr, _ := lookupEthAddress(ctx, smsg.Message.From, sa)
	toEthAddr, _ := lookupEthAddress(ctx, smsg.Message.To, sa)

	toAddr := &toEthAddr
	input := smsg.Message.Params
	var err error
	// Check to see if we need to decode as contract deployment.
	// We don't need to resolve the to address, because there's only one form (an ID).
	if smsg.Message.To == builtintypes.EthereumAddressManagerActorAddr {
		switch smsg.Message.Method {
		case builtintypes.MethodsEAM.Create:
			toAddr = nil
			var params eam.CreateParams
			err = params.UnmarshalCBOR(bytes.NewReader(smsg.Message.Params))
			input = params.Initcode
		case builtintypes.MethodsEAM.Create2:
			toAddr = nil
			var params eam.Create2Params
			err = params.UnmarshalCBOR(bytes.NewReader(smsg.Message.Params))
			input = params.Initcode
		case builtintypes.MethodsEAM.CreateExternal:
			toAddr = nil
			var params abi.CborBytes
			err = params.UnmarshalCBOR(bytes.NewReader(smsg.Message.Params))
			input = []byte(params)
		}
		if err != nil {
			return ethtypes.EthTx{}, err
		}
	}
	// Otherwise, try to decode as a cbor byte array.
	// TODO: Actually check if this is an ethereum call. This code will work for demo purposes, but is not correct.
	if toAddr != nil {
		if decodedParams, err := cbg.ReadByteArray(bytes.NewReader(smsg.Message.Params), uint64(len(smsg.Message.Params))); err == nil {
			input = decodedParams
		}
	}

	r, s, v, err := ethtypes.RecoverSignature(smsg.Signature)
	if err != nil {
		// we don't want to return error if the message is not an Eth tx
		r, s, v = ethtypes.EthBigIntZero, ethtypes.EthBigIntZero, ethtypes.EthBigIntZero
	}

	tx := ethtypes.EthTx{
		Nonce:                ethtypes.EthUint64(smsg.Message.Nonce),
		ChainID:              ethtypes.EthUint64(build.Eip155ChainId),
		From:                 fromEthAddr,
		To:                   toAddr,
		Value:                ethtypes.EthBigInt(smsg.Message.Value),
		Type:                 ethtypes.EthUint64(2),
		Input:                input,
		Gas:                  ethtypes.EthUint64(smsg.Message.GasLimit),
		MaxFeePerGas:         ethtypes.EthBigInt(smsg.Message.GasFeeCap),
		MaxPriorityFeePerGas: ethtypes.EthBigInt(smsg.Message.GasPremium),
		V:                    v,
		R:                    r,
		S:                    s,
	}

	// This is an eth tx
	if smsg.Signature.Type == crypto.SigTypeDelegated {
		tx.Hash, err = tx.TxHash()
		if err != nil {
			return tx, err
		}
	} else if smsg.Signature.Type == crypto.SigTypeUnknown { // BLS Filecoin message
		tx.Hash, err = ethtypes.EthHashFromCid(smsg.Message.Cid())
		if err != nil {
			return tx, err
		}
	} else { // Secp Filecoin Message
		tx.Hash, err = ethtypes.EthHashFromCid(smsg.Cid())
		if err != nil {
			return tx, err
		}
	}

	return tx, nil
}

// newEthTxFromFilecoinMessageLookup creates an ethereum transaction from filecoin message lookup. If a negative txIdx is passed
// into the function, it looksup the transaction index of the message in the tipset, otherwise it uses the txIdx passed into the
// function
func newEthTxFromFilecoinMessageLookup(ctx context.Context, msgLookup *api.MsgLookup, txIdx int, cs *store.ChainStore, sa StateAPI) (ethtypes.EthTx, error) {
	if msgLookup == nil {
		return ethtypes.EthTx{}, fmt.Errorf("msg does not exist")
	}

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

	smsg, err := cs.GetSignedMessage(ctx, msgLookup.Message)
	if err != nil {
		// We couldn't find the signed message, it might be a BLS message, so search for a regular message.
		msg, err := cs.GetMessage(ctx, msgLookup.Message)
		if err != nil {
			return ethtypes.EthTx{}, err
		}
		smsg = &types.SignedMessage{
			Message: *msg,
			Signature: crypto.Signature{
				Type: crypto.SigTypeUnknown,
				Data: nil,
			},
		}
	}

	tx, err := NewEthTxFromFilecoinMessage(ctx, smsg, sa)
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

func newEthTxReceipt(ctx context.Context, tx ethtypes.EthTx, lookup *api.MsgLookup, replay *api.InvocResult, events []types.Event, sa StateAPI) (api.EthTxReceipt, error) {
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

	if receipt.To == nil && lookup.Receipt.ExitCode.IsSuccess() {
		// Create and Create2 return the same things.
		var ret eam.CreateReturn
		if err := ret.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)); err != nil {
			return api.EthTxReceipt{}, xerrors.Errorf("failed to parse contract creation result: %w", err)
		}
		addr := ethtypes.EthAddress(ret.EthAddress)
		receipt.ContractAddress = &addr
	}

	if lookup.Receipt.ExitCode.IsSuccess() {
		receipt.Status = 1
	}
	if lookup.Receipt.ExitCode.IsError() {
		receipt.Status = 0
	}

	if len(events) > 0 {
		// TODO return a dummy non-zero bloom to signal that there are logs
		//  need to figure out how worth it is to populate with a real bloom
		//  should be feasible here since we are iterating over the logs anyway
		receipt.LogsBloom[255] = 0x01

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

			for _, entry := range evt.Entries {
				value := ethtypes.EthBytes(leftpad32(entry.Value)) // value has already been cbor-decoded but see https://github.com/filecoin-project/ref-fvm/issues/1345
				if entry.Key == ethtypes.EthTopic1 || entry.Key == ethtypes.EthTopic2 || entry.Key == ethtypes.EthTopic3 || entry.Key == ethtypes.EthTopic4 {
					l.Topics = append(l.Topics, value)
				} else {
					l.Data = value
				}
			}

			addr, err := address.NewIDAddress(uint64(evt.Emitter))
			if err != nil {
				return api.EthTxReceipt{}, xerrors.Errorf("failed to create ID address: %w", err)
			}

			l.Address, err = lookupEthAddress(ctx, addr, sa)
			if err != nil {
				return api.EthTxReceipt{}, xerrors.Errorf("failed to resolve Ethereum address: %w", err)
			}

			receipt.Logs = append(receipt.Logs, l)
		}
	}

	receipt.GasUsed = ethtypes.EthUint64(lookup.Receipt.GasUsed)

	// TODO: handle CumulativeGasUsed
	receipt.CumulativeGasUsed = ethtypes.EmptyEthInt

	effectiveGasPrice := big.Div(replay.GasCost.TotalCost, big.NewInt(lookup.Receipt.GasUsed))
	receipt.EffectiveGasPrice = ethtypes.EthBigInt(effectiveGasPrice)

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

			hash, err := EthTxHashFromSignedFilecoinMessage(ctx, smsg, m.StateAPI)
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

func WaitForMpoolUpdates(ctx context.Context, ch <-chan api.MpoolUpdate, manager *EthTxHashManager) {
	for {
		select {
		case <-ctx.Done():
			return
		case u := <-ch:
			if u.Type != api.MpoolAdd {
				continue
			}
			if u.Message.Signature.Type != crypto.SigTypeDelegated {
				continue
			}

			ethTx, err := NewEthTxFromFilecoinMessage(ctx, u.Message, manager.StateAPI)
			if err != nil {
				log.Errorf("error converting filecoin message to eth tx: %s", err)
			}

			err = manager.TransactionHashLookup.UpsertHash(ethTx.Hash, u.Message.Cid())
			if err != nil {
				log.Errorf("error inserting tx mapping to db: %s", err)
			}
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

// TODO we could also emit full EVM words from the EVM runtime, but not doing so
// makes the contract slightly cheaper (and saves storage bytes), at the expense
// of having to left pad in the API, which is a pretty acceptable tradeoff at
// face value. There may be other protocol implications to consider.
func leftpad32(orig []byte) []byte {
	needed := 32 - len(orig)
	if needed <= 0 {
		return orig
	}
	ret := make([]byte, 32)
	copy(ret[needed:], orig)
	return ret
}
