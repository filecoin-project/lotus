package full

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v8/eam"
	"github.com/filecoin-project/go-state-types/builtin/v8/evm"
	"github.com/filecoin-project/specs-actors/actors/builtin"

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
	// EthFeeHistory(ctx context.Context, blkCount string)
}

type EthEventAPI interface {
	EthGetLogs(ctx context.Context, filter *api.EthFilterSpec) (*api.EthFilterResult, error)
	EthGetFilterChanges(ctx context.Context, id api.EthFilterID) (*api.EthFilterResult, error)
	EthGetFilterLogs(ctx context.Context, id api.EthFilterID) (*api.EthFilterResult, error)
	EthNewFilter(ctx context.Context, filter *api.EthFilterSpec) (api.EthFilterID, error)
	EthNewBlockFilter(ctx context.Context) (api.EthFilterID, error)
	EthNewPendingTransactionFilter(ctx context.Context) (api.EthFilterID, error)
	EthUninstallFilter(ctx context.Context, id api.EthFilterID) (bool, error)
	EthSubscribe(ctx context.Context, eventTypes []string, params api.EthSubscriptionParams) (api.EthSubscriptionResponse, error)
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
	EventFilterManager   *filter.EventFilterManager
	TipSetFilterManager  *filter.TipSetFilterManager
	MemPoolFilterManager *filter.MemPoolFilterManager
	FilterStore          filter.FilterStore
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
	return a.ethBlockFromFilecoinTipSet(ctx, ts, fullTxInfo)
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
	return a.ethBlockFromFilecoinTipSet(ctx, ts, fullTxInfo)
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

	tx, err := a.ethTxFromFilecoinMessageLookup(ctx, msgLookup)
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

	tx, err := a.ethTxFromFilecoinMessageLookup(ctx, msgLookup)
	if err != nil {
		return nil, nil
	}

	replay, err := a.StateAPI.StateReplay(ctx, types.EmptyTSK, cid)
	if err != nil {
		return nil, nil
	}

	receipt, err := api.NewEthTxReceipt(tx, msgLookup, replay)
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
		block, err := a.ethBlockFromFilecoinTipSet(ctx, ts, false)
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
	gasPremium, err := a.GasAPI.GasEstimateGasPremium(ctx, 0, builtin.SystemActorAddr, 10000, types.EmptyTSK)
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
	return api.EthHashFromCid(cid)
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

func (a *EthModule) ethBlockFromFilecoinTipSet(ctx context.Context, ts *types.TipSet, fullTxInfo bool) (api.EthBlock, error) {
	parent, err := a.Chain.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return api.EthBlock{}, err
	}
	parentKeyCid, err := parent.Key().Cid()
	if err != nil {
		return api.EthBlock{}, err
	}
	parentBlkHash, err := api.EthHashFromCid(parentKeyCid)
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
			if err != nil {
				return api.EthBlock{}, nil
			}
			gasUsed += msgLookup.Receipt.GasUsed

			if fullTxInfo {
				tx, err := a.ethTxFromFilecoinMessageLookup(ctx, msgLookup)
				if err != nil {
					return api.EthBlock{}, nil
				}
				block.Transactions = append(block.Transactions, tx)
			} else {
				hash, err := api.EthHashFromCid(msg.Cid())
				if err != nil {
					return api.EthBlock{}, err
				}
				block.Transactions = append(block.Transactions, hash.String())
			}
		}
	}

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

func (a *EthModule) ethTxFromFilecoinMessageLookup(ctx context.Context, msgLookup *api.MsgLookup) (api.EthTx, error) {
	if msgLookup == nil {
		return api.EthTx{}, fmt.Errorf("msg does not exist")
	}
	cid := msgLookup.Message
	txHash, err := api.EthHashFromCid(cid)
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

	blkHash, err := api.EthHashFromCid(parentTsCid)
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

func (e *EthEvent) EthGetLogs(ctx context.Context, filter *api.EthFilterSpec) (*api.EthFilterResult, error) {
	// TODO: implement EthGetLogs
	return nil, api.ErrNotSupported
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

func (e *EthEvent) EthNewFilter(ctx context.Context, filter *api.EthFilterSpec) (api.EthFilterID, error) {
	// TODO: implement EthNewFilter
	return "", api.ErrNotSupported
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

func (e *EthEvent) EthSubscribe(ctx context.Context, eventTypes []string, params api.EthSubscriptionParams) (api.EthSubscriptionResponse, error) {
	// TODO: implement EthSubscribe
	return api.EthSubscriptionResponse{}, api.ErrNotSupported
}

func (e *EthEvent) EthUnsubscribe(ctx context.Context, id api.EthSubscriptionID) (bool, error) {
	// TODO: implement EthUnsubscribe
	return false, api.ErrNotSupported
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
	// TODO: implement ethFilterResultFromEvents
	return nil, nil
}

func ethFilterResultFromTipSets(tsks []types.TipSetKey) (*api.EthFilterResult, error) {
	res := &api.EthFilterResult{}

	for _, tsk := range tsks {
		c, err := tsk.Cid()
		if err != nil {
			return nil, err
		}
		hash, err := api.EthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		res.NewBlockHashes = append(res.NewBlockHashes, hash)
	}

	return res, nil
}

func ethFilterResultFromMessages(cs []cid.Cid) (*api.EthFilterResult, error) {
	res := &api.EthFilterResult{}

	for _, c := range cs {
		hash, err := api.EthHashFromCid(c)
		if err != nil {
			return nil, err
		}

		res.NewTransactionHashes = append(res.NewTransactionHashes, hash)
	}

	return res, nil
}
