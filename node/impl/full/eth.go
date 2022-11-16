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

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type EthModuleAPI interface {
	EthBlockNumber(ctx context.Context) (api.EthUint64, error)
	EthAccounts(ctx context.Context) ([]api.EthAddress, error)
	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum api.EthUint64) (api.EthUint64, error)
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash api.EthHash) (api.EthUint64, error)
	EthGetBlockByHash(ctx context.Context, blkHash api.EthHash, fullTxInfo bool) (api.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (api.EthBlock, error)
	EthGetTransactionByHash(ctx context.Context, txHash *api.EthHash) (*api.EthTx, error)
	EthGetTransactionCount(ctx context.Context, sender api.EthAddress, blkOpt string) (api.EthUint64, error)
	EthGetTransactionReceipt(ctx context.Context, txHash api.EthHash) (*api.EthTxReceipt, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash api.EthHash, txIndex api.EthUint64) (api.EthTx, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum api.EthUint64, txIndex api.EthUint64) (api.EthTx, error)
	EthGetCode(ctx context.Context, address api.EthAddress, blkOpt string) (api.EthBytes, error)
	EthGetStorageAt(ctx context.Context, address api.EthAddress, position api.EthBytes, blkParam string) (api.EthBytes, error)
	EthGetBalance(ctx context.Context, address api.EthAddress, blkParam string) (api.EthBigInt, error)
	EthFeeHistory(ctx context.Context, blkCount api.EthUint64, newestBlk string, rewardPercentiles []int64) (api.EthFeeHistory, error)
	EthChainId(ctx context.Context) (api.EthUint64, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (api.EthUint64, error)
	EthGasPrice(ctx context.Context) (api.EthBigInt, error)
	EthEstimateGas(ctx context.Context, tx api.EthCall) (api.EthUint64, error)
	EthCall(ctx context.Context, tx api.EthCall, blkParam string) (api.EthBytes, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (api.EthBigInt, error)
	EthSendRawTransaction(ctx context.Context, rawTx api.EthBytes) (api.EthHash, error)
}

type EthEventAPI interface {
	EthGetLogs(ctx context.Context, filter *api.EthFilterSpec) (*api.EthFilterResult, error)
	EthGetFilterChanges(ctx context.Context, id api.EthFilterID) (*api.EthFilterResult, error)
	EthGetFilterLogs(ctx context.Context, id api.EthFilterID) (*api.EthFilterResult, error)
	EthNewFilter(ctx context.Context, filter *api.EthFilterSpec) (api.EthFilterID, error)
	EthNewBlockFilter(ctx context.Context) (api.EthFilterID, error)
	EthNewPendingTransactionFilter(ctx context.Context) (api.EthFilterID, error)
	EthUninstallFilter(ctx context.Context, id api.EthFilterID) (bool, error)
	EthSubscribe(ctx context.Context, eventType string, params *api.EthSubscriptionParams) (<-chan api.EthSubscriptionResponse, error)
	EthUnsubscribe(ctx context.Context, id api.EthSubscriptionID) (bool, error)
}

var (
	_ EthModuleAPI = *new(api.FullNode)
	_ EthEventAPI  = *new(api.FullNode)
)

// EthModule provides a default implementation of EthModuleAPI.
// It can be swapped out with another implementation through Dependency
// Injection (for example with a thin RPC client).
type EthModule struct {
	fx.In

	Chain        *store.ChainStore
	Mpool        *messagepool.MessagePool
	StateManager *stmgr.StateManager

	ChainAPI
	MpoolAPI
	StateAPI
}

var _ EthModuleAPI = (*EthModule)(nil)

type EthEvent struct {
	EthModuleAPI
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

func (a *EthModule) EthBlockNumber(context.Context) (api.EthUint64, error) {
	height := a.Chain.GetHeaviestTipSet().Height()
	return api.EthUint64(height), nil
}

func (a *EthModule) EthAccounts(context.Context) ([]api.EthAddress, error) {
	// The lotus node is not expected to hold manage accounts, so we'll always return an empty array
	return []api.EthAddress{}, nil
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

func (a *EthModule) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum api.EthUint64) (api.EthUint64, error) {
	ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(blkNum), nil, false)
	if err != nil {
		return api.EthUint64(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}

	count, err := a.countTipsetMsgs(ctx, ts)
	return api.EthUint64(count), err
}

func (a *EthModule) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash api.EthHash) (api.EthUint64, error) {
	ts, err := a.Chain.GetTipSetByCid(ctx, blkHash.ToCid())
	if err != nil {
		return api.EthUint64(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	count, err := a.countTipsetMsgs(ctx, ts)
	return api.EthUint64(count), err
}

func (a *EthModule) EthGetBlockByHash(ctx context.Context, blkHash api.EthHash, fullTxInfo bool) (api.EthBlock, error) {
	ts, err := a.Chain.GetTipSetByCid(ctx, blkHash.ToCid())
	if err != nil {
		return api.EthBlock{}, xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	return a.newEthBlockFromFilecoinTipSet(ctx, ts, fullTxInfo)
}

func (a *EthModule) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (api.EthBlock, error) {
	var num api.EthUint64
	err := num.UnmarshalJSON([]byte(`"` + blkNum + `"`))
	if err != nil {
		num = api.EthUint64(a.Chain.GetHeaviestTipSet().Height())
	}

	ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(num), nil, false)
	if err != nil {
		return api.EthBlock{}, xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	return a.newEthBlockFromFilecoinTipSet(ctx, ts, fullTxInfo)
}

func (a *EthModule) EthGetTransactionByHash(ctx context.Context, txHash *api.EthHash) (*api.EthTx, error) {
	// Ethereum's behavior is to return null when the txHash is invalid, so we use nil to check if txHash is valid
	if txHash == nil {
		return nil, nil
	}

	cid := txHash.ToCid()

	msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, cid, api.LookbackNoLimit, true)
	if err != nil {
		return nil, nil
	}

	tx, err := a.newEthTxFromFilecoinMessageLookup(ctx, msgLookup)
	if err != nil {
		return nil, nil
	}
	return &tx, nil
}

func (a *EthModule) EthGetTransactionCount(ctx context.Context, sender api.EthAddress, blkParam string) (api.EthUint64, error) {
	addr, err := sender.ToFilecoinAddress()
	if err != nil {
		return api.EthUint64(0), nil
	}
	nonce, err := a.Mpool.GetNonce(ctx, addr, types.EmptyTSK)
	if err != nil {
		return api.EthUint64(0), nil
	}
	return api.EthUint64(nonce), nil
}

func (a *EthModule) EthGetTransactionReceipt(ctx context.Context, txHash api.EthHash) (*api.EthTxReceipt, error) {
	cid := txHash.ToCid()

	msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, cid, api.LookbackNoLimit, true)
	if err != nil {
		return nil, nil
	}

	tx, err := a.newEthTxFromFilecoinMessageLookup(ctx, msgLookup)
	if err != nil {
		return nil, nil
	}

	replay, err := a.StateAPI.StateReplay(ctx, types.EmptyTSK, cid)
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

	receipt, err := a.newEthTxReceipt(ctx, tx, msgLookup, replay, events)
	if err != nil {
		return nil, nil
	}

	return &receipt, nil
}

func (a *EthModule) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash api.EthHash, txIndex api.EthUint64) (api.EthTx, error) {
	return api.EthTx{}, nil
}

func (a *EthModule) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum api.EthUint64, txIndex api.EthUint64) (api.EthTx, error) {
	return api.EthTx{}, nil
}

// EthGetCode returns string value of the compiled bytecode
func (a *EthModule) EthGetCode(ctx context.Context, ethAddr api.EthAddress, blkOpt string) (api.EthBytes, error) {
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

func (a *EthModule) EthGetStorageAt(ctx context.Context, ethAddr api.EthAddress, position api.EthBytes, blkParam string) (api.EthBytes, error) {
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

func (a *EthModule) EthGetBalance(ctx context.Context, address api.EthAddress, blkParam string) (api.EthBigInt, error) {
	filAddr, err := address.ToFilecoinAddress()
	if err != nil {
		return api.EthBigInt{}, err
	}

	actor, err := a.StateGetActor(ctx, filAddr, types.EmptyTSK)
	if xerrors.Is(err, types.ErrActorNotFound) {
		return api.EthBigIntZero, nil
	} else if err != nil {
		return api.EthBigInt{}, err
	}

	return api.EthBigInt{Int: actor.Balance.Int}, nil
}

func (a *EthModule) EthChainId(ctx context.Context) (api.EthUint64, error) {
	return api.EthUint64(build.Eip155ChainId), nil
}

func (a *EthModule) EthFeeHistory(ctx context.Context, blkCount api.EthUint64, newestBlkNum string, rewardPercentiles []int64) (api.EthFeeHistory, error) {
	if blkCount > 1024 {
		return api.EthFeeHistory{}, fmt.Errorf("block count should be smaller than 1024")
	}

	newestBlkHeight := uint64(a.Chain.GetHeaviestTipSet().Height())

	// TODO https://github.com/filecoin-project/ref-fvm/issues/1016
	var blkNum api.EthUint64
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
		return api.EthFeeHistory{}, fmt.Errorf("cannot load find block height: %v", newestBlkHeight)
	}

	// FIXME: baseFeePerGas should include the next block after the newest of the returned range, because this
	// can be inferred from the newest block. we use the newest block's baseFeePerGas for now but need to fix it
	// In other words, due to deferred execution, we might not be returning the most useful value here for the client.
	baseFeeArray := []api.EthBigInt{api.EthBigInt(ts.Blocks()[0].ParentBaseFee)}
	gasUsedRatioArray := []float64{}

	for ts.Height() >= abi.ChainEpoch(oldestBlkHeight) {
		// Unfortunately we need to rebuild the full message view so we can
		// totalize gas used in the tipset.
		block, err := a.newEthBlockFromFilecoinTipSet(ctx, ts, false)
		if err != nil {
			return api.EthFeeHistory{}, fmt.Errorf("cannot create eth block: %v", err)
		}

		// both arrays should be reversed at the end
		baseFeeArray = append(baseFeeArray, api.EthBigInt(ts.Blocks()[0].ParentBaseFee))
		gasUsedRatioArray = append(gasUsedRatioArray, float64(block.GasUsed)/float64(build.BlockGasLimit))

		parentTsKey := ts.Parents()
		ts, err = a.Chain.LoadTipSet(ctx, parentTsKey)
		if err != nil {
			return api.EthFeeHistory{}, fmt.Errorf("cannot load tipset key: %v", parentTsKey)
		}
	}

	// Reverse the arrays; we collected them newest to oldest; the client expects oldest to newest.

	for i, j := 0, len(baseFeeArray)-1; i < j; i, j = i+1, j-1 {
		baseFeeArray[i], baseFeeArray[j] = baseFeeArray[j], baseFeeArray[i]
	}
	for i, j := 0, len(gasUsedRatioArray)-1; i < j; i, j = i+1, j-1 {
		gasUsedRatioArray[i], gasUsedRatioArray[j] = gasUsedRatioArray[j], gasUsedRatioArray[i]
	}

	return api.EthFeeHistory{
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

func (a *EthModule) EthProtocolVersion(ctx context.Context) (api.EthUint64, error) {
	height := a.Chain.GetHeaviestTipSet().Height()
	return api.EthUint64(a.StateManager.GetNetworkVersion(ctx, height)), nil
}

func (a *EthModule) EthMaxPriorityFeePerGas(ctx context.Context) (api.EthBigInt, error) {
	gasPremium, err := a.GasAPI.GasEstimateGasPremium(ctx, 0, builtinactors.SystemActorAddr, 10000, types.EmptyTSK)
	if err != nil {
		return api.EthBigInt(big.Zero()), err
	}
	return api.EthBigInt(gasPremium), nil
}

func (a *EthModule) EthGasPrice(ctx context.Context) (api.EthBigInt, error) {
	// According to Geth's implementation, eth_gasPrice should return base + tip
	// Ref: https://github.com/ethereum/pm/issues/328#issuecomment-853234014

	ts := a.Chain.GetHeaviestTipSet()
	baseFee := ts.Blocks()[0].ParentBaseFee

	premium, err := a.EthMaxPriorityFeePerGas(ctx)
	if err != nil {
		return api.EthBigInt(big.Zero()), nil
	}

	gasPrice := big.Add(baseFee, big.Int(premium))
	return api.EthBigInt(gasPrice), nil
}

func (a *EthModule) EthSendRawTransaction(ctx context.Context, rawTx api.EthBytes) (api.EthHash, error) {
	txArgs, err := api.ParseEthTxArgs(rawTx)
	if err != nil {
		return api.EmptyEthHash, err
	}

	smsg, err := txArgs.ToSignedMessage()
	if err != nil {
		return api.EmptyEthHash, err
	}

	cid, err := a.MpoolAPI.MpoolPush(ctx, smsg)
	if err != nil {
		return api.EmptyEthHash, err
	}
	return api.NewEthHashFromCid(cid)
}

func (a *EthModule) ethCallToFilecoinMessage(ctx context.Context, tx api.EthCall) (*types.Message, error) {
	var err error
	var from address.Address
	if tx.From == nil {
		// Send from the filecoin "system" address.
		from, err = (api.EthAddress{}).ToFilecoinAddress()
		if err != nil {
			return nil, fmt.Errorf("failed to construct the ethereum system address: %w", err)
		}
	} else {
		// The from address must be translatable to an f4 address.
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

		nonce, err := a.Mpool.GetNonce(ctx, from, types.EmptyTSK)
		if err != nil {
			nonce = 0 // assume a zero nonce on error (e.g. sender doesn't exist).
		}

		params2, err := actors.SerializeParams(&eam.CreateParams{
			Initcode: tx.Data,
			Nonce:    nonce,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to serialize Create params: %w", err)
		}
		params = params2
		method = builtintypes.MethodsEAM.Create
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

func (a *EthModule) applyMessage(ctx context.Context, msg *types.Message) (res *api.InvocResult, err error) {
	ts := a.Chain.GetHeaviestTipSet()

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
		return nil, xerrors.Errorf("message execution failed: exit %s, reason: %s", res.MsgRct.ExitCode, res.Error)
	}
	return res, nil
}

func (a *EthModule) EthEstimateGas(ctx context.Context, tx api.EthCall) (api.EthUint64, error) {
	msg, err := a.ethCallToFilecoinMessage(ctx, tx)
	if err != nil {
		return api.EthUint64(0), err
	}

	// Set the gas limit to the zero sentinel value, which makes
	// gas estimation actually run.
	msg.GasLimit = 0

	msg, err = a.GasAPI.GasEstimateMessageGas(ctx, msg, nil, types.EmptyTSK)
	if err != nil {
		return api.EthUint64(0), err
	}

	return api.EthUint64(msg.GasLimit), nil
}

func (a *EthModule) EthCall(ctx context.Context, tx api.EthCall, blkParam string) (api.EthBytes, error) {
	msg, err := a.ethCallToFilecoinMessage(ctx, tx)
	if err != nil {
		return nil, err
	}

	invokeResult, err := a.applyMessage(ctx, msg)
	if err != nil {
		return nil, err
	}
	if len(invokeResult.MsgRct.Return) > 0 {
		return cbg.ReadByteArray(bytes.NewReader(invokeResult.MsgRct.Return), uint64(len(invokeResult.MsgRct.Return)))
	}
	return api.EthBytes{}, nil
}

func (a *EthModule) newEthBlockFromFilecoinTipSet(ctx context.Context, ts *types.TipSet, fullTxInfo bool) (api.EthBlock, error) {
	parent, err := a.Chain.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return api.EthBlock{}, err
	}
	parentKeyCid, err := parent.Key().Cid()
	if err != nil {
		return api.EthBlock{}, err
	}
	parentBlkHash, err := api.NewEthHashFromCid(parentKeyCid)
	if err != nil {
		return api.EthBlock{}, err
	}

	blkCid, err := ts.Key().Cid()
	if err != nil {
		return api.EthBlock{}, err
	}
	blkHash, err := api.NewEthHashFromCid(blkCid)
	if err != nil {
		return api.EthBlock{}, err
	}

	blkMsgs, err := a.Chain.BlockMsgsForTipset(ctx, ts)
	if err != nil {
		return api.EthBlock{}, xerrors.Errorf("error loading messages for tipset: %v: %w", ts, err)
	}

	block := api.NewEthBlock()

	// this seems to be a very expensive way to get gasUsed of the block. may need to find an efficient way to do it
	gasUsed := int64(0)
	for _, blkMsg := range blkMsgs {
		for _, msg := range append(blkMsg.BlsMessages, blkMsg.SecpkMessages...) {
			msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, msg.Cid(), api.LookbackNoLimit, true)
			if err != nil || msgLookup == nil {
				return api.EthBlock{}, nil
			}
			gasUsed += msgLookup.Receipt.GasUsed

			if fullTxInfo {
				tx, err := a.newEthTxFromFilecoinMessageLookup(ctx, msgLookup)
				if err != nil {
					return api.EthBlock{}, nil
				}
				block.Transactions = append(block.Transactions, tx)
			} else {
				hash, err := api.NewEthHashFromCid(msg.Cid())
				if err != nil {
					return api.EthBlock{}, err
				}
				block.Transactions = append(block.Transactions, hash.String())
			}
		}
	}

	block.Hash = blkHash
	block.Number = api.EthUint64(ts.Height())
	block.ParentHash = parentBlkHash
	block.Timestamp = api.EthUint64(ts.Blocks()[0].Timestamp)
	block.BaseFeePerGas = api.EthBigInt{Int: ts.Blocks()[0].ParentBaseFee.Int}
	block.GasUsed = api.EthUint64(gasUsed)
	return block, nil
}

// lookupEthAddress makes its best effort at finding the Ethereum address for a
// Filecoin address. It does the following:
//
//  1. If the supplied address is an f410 address, we return its payload as the EthAddress.
//  2. Otherwise (f0, f1, f2, f3), we look up the actor on the state tree. If it has a predictable address, we return it if it's f410 address.
//  3. Otherwise, we fall back to returning a masked ID Ethereum address. If the supplied address is an f0 address, we
//     use that ID to form the masked ID address.
//  4. Otherwise, we fetch the actor's ID from the state tree and form the masked ID with it.
func (a *EthModule) lookupEthAddress(ctx context.Context, addr address.Address) (api.EthAddress, error) {
	// Attempt to convert directly.
	if ethAddr, ok, err := api.TryEthAddressFromFilecoinAddress(addr, false); err != nil {
		return api.EthAddress{}, err
	} else if ok {
		return ethAddr, nil
	}

	// Lookup on the target actor.
	actor, err := a.StateAPI.StateGetActor(ctx, addr, types.EmptyTSK)
	if err != nil {
		return api.EthAddress{}, err
	}
	if actor.Address != nil {
		if ethAddr, ok, err := api.TryEthAddressFromFilecoinAddress(*actor.Address, false); err != nil {
			return api.EthAddress{}, err
		} else if ok {
			return ethAddr, nil
		}
	}

	// Check if we already have an ID addr, and use it if possible.
	if ethAddr, ok, err := api.TryEthAddressFromFilecoinAddress(addr, true); err != nil {
		return api.EthAddress{}, err
	} else if ok {
		return ethAddr, nil
	}

	// Otherwise, resolve the ID addr.
	idAddr, err := a.StateAPI.StateLookupID(ctx, addr, types.EmptyTSK)
	if err != nil {
		return api.EthAddress{}, err
	}
	return api.EthAddressFromFilecoinAddress(idAddr)
}

func (a *EthModule) newEthTxFromFilecoinMessageLookup(ctx context.Context, msgLookup *api.MsgLookup) (api.EthTx, error) {
	if msgLookup == nil {
		return api.EthTx{}, fmt.Errorf("msg does not exist")
	}
	cid := msgLookup.Message
	txHash, err := api.NewEthHashFromCid(cid)
	if err != nil {
		return api.EthTx{}, err
	}

	ts, err := a.Chain.LoadTipSet(ctx, msgLookup.TipSet)
	if err != nil {
		return api.EthTx{}, err
	}

	// This tx is located in the parent tipset
	parentTs, err := a.Chain.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return api.EthTx{}, err
	}

	parentTsCid, err := parentTs.Key().Cid()
	if err != nil {
		return api.EthTx{}, err
	}

	blkHash, err := api.NewEthHashFromCid(parentTsCid)
	if err != nil {
		return api.EthTx{}, err
	}

	msg, err := a.ChainAPI.ChainGetMessage(ctx, msgLookup.Message)
	if err != nil {
		return api.EthTx{}, err
	}

	fromEthAddr, err := a.lookupEthAddress(ctx, msg.From)
	if err != nil {
		return api.EthTx{}, err
	}

	toEthAddr, err := a.lookupEthAddress(ctx, msg.To)
	if err != nil {
		return api.EthTx{}, err
	}

	toAddr := &toEthAddr
	input := msg.Params
	// Check to see if we need to decode as contract deployment.
	// We don't need to resolve the to address, because there's only one form (an ID).
	if msg.To == builtintypes.EthereumAddressManagerActorAddr {
		switch msg.Method {
		case builtintypes.MethodsEAM.Create:
			toAddr = nil
			var params eam.CreateParams
			err = params.UnmarshalCBOR(bytes.NewReader(msg.Params))
			input = params.Initcode
		case builtintypes.MethodsEAM.Create2:
			toAddr = nil
			var params eam.Create2Params
			err = params.UnmarshalCBOR(bytes.NewReader(msg.Params))
			input = params.Initcode
		}
		if err != nil {
			return api.EthTx{}, err
		}
	}
	// Otherwise, try to decode as a cbor byte array.
	// TODO: Actually check if this is an ethereum call. This code will work for demo purposes, but is not correct.
	if toAddr != nil {
		if decodedParams, err := cbg.ReadByteArray(bytes.NewReader(msg.Params), uint64(len(msg.Params))); err == nil {
			input = decodedParams
		}
	}

	tx := api.EthTx{
		ChainID:              api.EthUint64(build.Eip155ChainId),
		Hash:                 txHash,
		BlockHash:            blkHash,
		BlockNumber:          api.EthUint64(parentTs.Height()),
		From:                 fromEthAddr,
		To:                   toAddr,
		Value:                api.EthBigInt(msg.Value),
		Type:                 api.EthUint64(2),
		Gas:                  api.EthUint64(msg.GasLimit),
		MaxFeePerGas:         api.EthBigInt(msg.GasFeeCap),
		MaxPriorityFeePerGas: api.EthBigInt(msg.GasPremium),
		V:                    api.EthBytes{},
		R:                    api.EthBytes{},
		S:                    api.EthBytes{},
		Input:                input,
	}
	return tx, nil
}

func (a *EthModule) newEthTxReceipt(ctx context.Context, tx api.EthTx, lookup *api.MsgLookup, replay *api.InvocResult, events []types.Event) (api.EthTxReceipt, error) {
	receipt := api.EthTxReceipt{
		TransactionHash:  tx.Hash,
		TransactionIndex: tx.TransactionIndex,
		BlockHash:        tx.BlockHash,
		BlockNumber:      tx.BlockNumber,
		From:             tx.From,
		To:               tx.To,
		StateRoot:        api.EmptyEthHash,
		LogsBloom:        []byte{0},
	}

	if receipt.To == nil && lookup.Receipt.ExitCode.IsSuccess() {
		// Create and Create2 return the same things.
		var ret eam.CreateReturn
		if err := ret.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)); err != nil {
			return api.EthTxReceipt{}, xerrors.Errorf("failed to parse contract creation result: %w", err)
		}
		addr := api.EthAddress(ret.EthAddress)
		receipt.ContractAddress = &addr
	}

	if lookup.Receipt.ExitCode.IsSuccess() {
		receipt.Status = 1
	}
	if lookup.Receipt.ExitCode.IsError() {
		receipt.Status = 0
	}

	if len(events) > 0 {
		receipt.Logs = make([]api.EthLog, 0, len(events))
		for i, evt := range events {
			l := api.EthLog{
				Removed:          false,
				LogIndex:         api.EthUint64(i),
				TransactionIndex: tx.TransactionIndex,
				TransactionHash:  tx.Hash,
				BlockHash:        tx.BlockHash,
				BlockNumber:      tx.BlockNumber,
			}

			for _, entry := range evt.Entries {
				hash := api.EthHashData(entry.Value)
				if entry.Key == api.EthTopic1 || entry.Key == api.EthTopic2 || entry.Key == api.EthTopic3 || entry.Key == api.EthTopic4 {
					l.Topics = append(l.Topics, hash)
				} else {
					l.Data = append(l.Data, hash)
				}
			}

			addr, err := address.NewIDAddress(uint64(evt.Emitter))
			if err != nil {
				return api.EthTxReceipt{}, xerrors.Errorf("failed to create ID address: %w", err)
			}

			l.Address, err = a.lookupEthAddress(ctx, addr)
			if err != nil {
				return api.EthTxReceipt{}, xerrors.Errorf("failed to resolve Ethereum address: %w", err)
			}

			receipt.Logs = append(receipt.Logs, l)
		}
	}

	receipt.GasUsed = api.EthUint64(lookup.Receipt.GasUsed)

	// TODO: handle CumulativeGasUsed
	receipt.CumulativeGasUsed = api.EmptyEthInt

	effectiveGasPrice := big.Div(replay.GasCost.TotalCost, big.NewInt(lookup.Receipt.GasUsed))
	receipt.EffectiveGasPrice = api.EthBigInt(effectiveGasPrice)

	return receipt, nil
}

func (e *EthEvent) EthGetLogs(ctx context.Context, filterSpec *api.EthFilterSpec) (*api.EthFilterResult, error) {
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

	return ethFilterResultFromEvents(ces)
}

func (e *EthEvent) EthGetFilterChanges(ctx context.Context, id api.EthFilterID) (*api.EthFilterResult, error) {
	if e.FilterStore == nil {
		return nil, api.ErrNotSupported
	}

	f, err := e.FilterStore.Get(ctx, string(id))
	if err != nil {
		return nil, err
	}

	switch fc := f.(type) {
	case filterEventCollector:
		return ethFilterResultFromEvents(fc.TakeCollectedEvents(ctx))
	case filterTipSetCollector:
		return ethFilterResultFromTipSets(fc.TakeCollectedTipSets(ctx))
	case filterMessageCollector:
		return ethFilterResultFromMessages(fc.TakeCollectedMessages(ctx))
	}

	return nil, xerrors.Errorf("unknown filter type")
}

func (e *EthEvent) EthGetFilterLogs(ctx context.Context, id api.EthFilterID) (*api.EthFilterResult, error) {
	if e.FilterStore == nil {
		return nil, api.ErrNotSupported
	}

	f, err := e.FilterStore.Get(ctx, string(id))
	if err != nil {
		return nil, err
	}

	switch fc := f.(type) {
	case filterEventCollector:
		return ethFilterResultFromEvents(fc.TakeCollectedEvents(ctx))
	}

	return nil, xerrors.Errorf("wrong filter type")
}

func (e *EthEvent) installEthFilterSpec(ctx context.Context, filterSpec *api.EthFilterSpec) (*filter.EventFilter, error) {
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
			epoch, err := api.EthUint64FromHex(*filterSpec.FromBlock)
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
			epoch, err := api.EthUint64FromHex(*filterSpec.FromBlock)
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
				return nil, xerrors.Errorf("invalid epoch range")
			}
		} else if minHeight >= 0 && maxHeight == -1 {
			// Here the client is looking for events between some time in the past and the current head
			ts := e.Chain.GetHeaviestTipSet()
			if ts.Height()-minHeight > e.MaxFilterHeightRange {
				return nil, xerrors.Errorf("invalid epoch range")
			}

		} else if minHeight >= 0 && maxHeight >= 0 {
			if minHeight > maxHeight || maxHeight-minHeight > e.MaxFilterHeightRange {
				return nil, xerrors.Errorf("invalid epoch range")
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
		// Ethereum topics are emitted using `LOG{0..4}` opcodes resulting in topics1..4
		key := fmt.Sprintf("topic%d", idx+1)
		keyvals := make([][]byte, len(vals))
		for i, v := range vals {
			keyvals[i] = v[:]
		}
		keys[key] = keyvals
	}

	return e.EventFilterManager.Install(ctx, minHeight, maxHeight, tipsetCid, addresses, keys)
}

func (e *EthEvent) EthNewFilter(ctx context.Context, filterSpec *api.EthFilterSpec) (api.EthFilterID, error) {
	if e.FilterStore == nil || e.EventFilterManager == nil {
		return "", api.ErrNotSupported
	}

	f, err := e.installEthFilterSpec(ctx, filterSpec)
	if err != nil {
		return "", err
	}

	if err := e.FilterStore.Add(ctx, f); err != nil {
		// Could not record in store, attempt to delete filter to clean up
		err2 := e.TipSetFilterManager.Remove(ctx, f.ID())
		if err2 != nil {
			return "", xerrors.Errorf("encountered error %v while removing new filter due to %v", err2, err)
		}

		return "", err
	}

	return api.EthFilterID(f.ID()), nil
}

func (e *EthEvent) EthNewBlockFilter(ctx context.Context) (api.EthFilterID, error) {
	if e.FilterStore == nil || e.TipSetFilterManager == nil {
		return "", api.ErrNotSupported
	}

	f, err := e.TipSetFilterManager.Install(ctx)
	if err != nil {
		return "", err
	}

	if err := e.FilterStore.Add(ctx, f); err != nil {
		// Could not record in store, attempt to delete filter to clean up
		err2 := e.TipSetFilterManager.Remove(ctx, f.ID())
		if err2 != nil {
			return "", xerrors.Errorf("encountered error %v while removing new filter due to %v", err2, err)
		}

		return "", err
	}

	return api.EthFilterID(f.ID()), nil
}

func (e *EthEvent) EthNewPendingTransactionFilter(ctx context.Context) (api.EthFilterID, error) {
	if e.FilterStore == nil || e.MemPoolFilterManager == nil {
		return "", api.ErrNotSupported
	}

	f, err := e.MemPoolFilterManager.Install(ctx)
	if err != nil {
		return "", err
	}

	if err := e.FilterStore.Add(ctx, f); err != nil {
		// Could not record in store, attempt to delete filter to clean up
		err2 := e.MemPoolFilterManager.Remove(ctx, f.ID())
		if err2 != nil {
			return "", xerrors.Errorf("encountered error %v while removing new filter due to %v", err2, err)
		}

		return "", err
	}

	return api.EthFilterID(f.ID()), nil
}

func (e *EthEvent) EthUninstallFilter(ctx context.Context, id api.EthFilterID) (bool, error) {
	if e.FilterStore == nil {
		return false, api.ErrNotSupported
	}

	f, err := e.FilterStore.Get(ctx, string(id))
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

func (e *EthEvent) EthSubscribe(ctx context.Context, eventType string, params *api.EthSubscriptionParams) (<-chan api.EthSubscriptionResponse, error) {
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
			_, _ = e.EthUnsubscribe(ctx, api.EthSubscriptionID(sub.id))
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
			_, _ = e.EthUnsubscribe(ctx, api.EthSubscriptionID(sub.id))
			return nil, err
		}
		sub.addFilter(ctx, f)
	default:
		return nil, xerrors.Errorf("unsupported event type: %s", eventType)
	}

	return sub.out, nil
}

func (e *EthEvent) EthUnsubscribe(ctx context.Context, id api.EthSubscriptionID) (bool, error) {
	if e.SubManager == nil {
		return false, api.ErrNotSupported
	}

	filters, err := e.SubManager.StopSubscription(ctx, string(id))
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
	TakeCollectedMessages(context.Context) []cid.Cid
}

type filterTipSetCollector interface {
	TakeCollectedTipSets(context.Context) []types.TipSetKey
}

func ethFilterResultFromEvents(evs []*filter.CollectedEvent) (*api.EthFilterResult, error) {
	res := &api.EthFilterResult{}
	for _, ev := range evs {
		log := api.EthLog{
			Removed:          ev.Reverted,
			LogIndex:         api.EthUint64(ev.EventIdx),
			TransactionIndex: api.EthUint64(ev.MsgIdx),
			BlockNumber:      api.EthUint64(ev.Height),
		}

		var err error

		for _, entry := range ev.Entries {
			hash := api.EthHashData(entry.Value)
			if entry.Key == api.EthTopic1 || entry.Key == api.EthTopic2 || entry.Key == api.EthTopic3 || entry.Key == api.EthTopic4 {
				log.Topics = append(log.Topics, hash)
			} else {
				log.Data = append(log.Data, hash)
			}
		}

		log.Address, err = api.EthAddressFromFilecoinAddress(ev.EmitterAddr)
		if err != nil {
			return nil, err
		}

		log.TransactionHash, err = api.NewEthHashFromCid(ev.MsgCid)
		if err != nil {
			return nil, err
		}

		c, err := ev.TipSetKey.Cid()
		if err != nil {
			return nil, err
		}
		log.BlockHash, err = api.NewEthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, log)
	}

	return res, nil
}

func ethFilterResultFromTipSets(tsks []types.TipSetKey) (*api.EthFilterResult, error) {
	res := &api.EthFilterResult{}

	for _, tsk := range tsks {
		c, err := tsk.Cid()
		if err != nil {
			return nil, err
		}
		hash, err := api.NewEthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, hash)
	}

	return res, nil
}

func ethFilterResultFromMessages(cs []cid.Cid) (*api.EthFilterResult, error) {
	res := &api.EthFilterResult{}

	for _, c := range cs {
		hash, err := api.NewEthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		res.Results = append(res.Results, hash)
	}

	return res, nil
}

type EthSubscriptionManager struct {
	EthModuleAPI
	mu   sync.Mutex
	subs map[string]*ethSubscription
}

func (e *EthSubscriptionManager) StartSubscription(ctx context.Context) (*ethSubscription, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, xerrors.Errorf("new uuid: %w", err)
	}

	ctx, quit := context.WithCancel(ctx)

	sub := &ethSubscription{
		EthModuleAPI: e.EthModuleAPI,
		id:           id.String(),
		in:           make(chan interface{}, 200),
		out:          make(chan api.EthSubscriptionResponse),
		quit:         quit,
	}

	e.mu.Lock()
	if e.subs == nil {
		e.subs = make(map[string]*ethSubscription)
	}
	e.subs[sub.id] = sub
	e.mu.Unlock()

	go sub.start(ctx)

	return sub, nil
}

func (e *EthSubscriptionManager) StopSubscription(ctx context.Context, id string) ([]filter.Filter, error) {
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
	EthModuleAPI
	id  string
	in  chan interface{}
	out chan api.EthSubscriptionResponse

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
			resp := api.EthSubscriptionResponse{
				SubscriptionID: api.EthSubscriptionID(e.id),
			}

			var err error
			switch vt := v.(type) {
			case *filter.CollectedEvent:
				resp.Result, err = ethFilterResultFromEvents([]*filter.CollectedEvent{vt})
			case *types.TipSet:
				// Sadly convoluted since the logic for conversion to eth block is long and buried away
				// in unexported methods of EthModule
				tsCid, err := vt.Key().Cid()
				if err != nil {
					break
				}

				hash, err := api.NewEthHashFromCid(tsCid)
				if err != nil {
					break
				}

				eb, err := e.EthGetBlockByHash(ctx, hash, true)
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
