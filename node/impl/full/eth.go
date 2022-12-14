package full

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

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
	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/eth"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type EthModuleAPI interface {
	EthBlockNumber(ctx context.Context) (eth.EthUint64, error)
	EthAccounts(ctx context.Context) ([]eth.EthAddress, error)
	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum eth.EthUint64) (eth.EthUint64, error)
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash eth.EthHash) (eth.EthUint64, error)
	EthGetBlockByHash(ctx context.Context, blkHash eth.EthHash, fullTxInfo bool) (eth.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (eth.EthBlock, error)
	EthGetTransactionByHash(ctx context.Context, txHash *eth.EthHash) (*eth.EthTx, error)
	EthGetTransactionCount(ctx context.Context, sender eth.EthAddress, blkOpt string) (eth.EthUint64, error)
	EthGetTransactionReceipt(ctx context.Context, txHash eth.EthHash) (*api.EthTxReceipt, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash eth.EthHash, txIndex eth.EthUint64) (eth.EthTx, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum eth.EthUint64, txIndex eth.EthUint64) (eth.EthTx, error)
	EthGetCode(ctx context.Context, address eth.EthAddress, blkOpt string) (eth.EthBytes, error)
	EthGetStorageAt(ctx context.Context, address eth.EthAddress, position eth.EthBytes, blkParam string) (eth.EthBytes, error)
	EthGetBalance(ctx context.Context, address eth.EthAddress, blkParam string) (eth.EthBigInt, error)
	EthFeeHistory(ctx context.Context, blkCount eth.EthUint64, newestBlk string, rewardPercentiles []float64) (eth.EthFeeHistory, error)
	EthChainId(ctx context.Context) (eth.EthUint64, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (eth.EthUint64, error)
	EthGasPrice(ctx context.Context) (eth.EthBigInt, error)
	EthEstimateGas(ctx context.Context, tx eth.EthCall) (eth.EthUint64, error)
	EthCall(ctx context.Context, tx eth.EthCall, blkParam string) (eth.EthBytes, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (eth.EthBigInt, error)
	EthSendRawTransaction(ctx context.Context, rawTx eth.EthBytes) (eth.EthHash, error)
}

var _ EthModuleAPI = *new(api.FullNode)

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

type EthAPI struct {
	fx.In

	Chain *store.ChainStore

	EthModuleAPI
}

func (a *EthModule) StateNetworkName(ctx context.Context) (dtypes.NetworkName, error) {
	return stmgr.GetNetworkName(ctx, a.StateManager, a.Chain.GetHeaviestTipSet().ParentState())
}

func (a *EthModule) EthBlockNumber(context.Context) (eth.EthUint64, error) {
	height := a.Chain.GetHeaviestTipSet().Height()
	return eth.EthUint64(height), nil
}

func (a *EthModule) EthAccounts(context.Context) ([]eth.EthAddress, error) {
	// The lotus node is not expected to hold manage accounts, so we'll always return an empty array
	return []eth.EthAddress{}, nil
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

func (a *EthModule) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum eth.EthUint64) (eth.EthUint64, error) {
	ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(blkNum), nil, false)
	if err != nil {
		return eth.EthUint64(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}

	count, err := a.countTipsetMsgs(ctx, ts)
	return eth.EthUint64(count), err
}

func (a *EthModule) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash eth.EthHash) (eth.EthUint64, error) {
	ts, err := a.Chain.GetTipSetByCid(ctx, blkHash.ToCid())
	if err != nil {
		return eth.EthUint64(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	count, err := a.countTipsetMsgs(ctx, ts)
	return eth.EthUint64(count), err
}

func (a *EthModule) EthGetBlockByHash(ctx context.Context, blkHash eth.EthHash, fullTxInfo bool) (eth.EthBlock, error) {
	ts, err := a.Chain.GetTipSetByCid(ctx, blkHash.ToCid())
	if err != nil {
		return eth.EthBlock{}, xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	return a.newEthBlockFromFilecoinTipSet(ctx, ts, fullTxInfo)
}

func (a *EthModule) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (eth.EthBlock, error) {
	typ, num, err := eth.ParseBlkNumOption(blkNum)
	if err != nil {
		return eth.EthBlock{}, fmt.Errorf("cannot parse block number: %v", err)
	}

	switch typ {
	case eth.BlkNumLatest:
		num = eth.EthUint64(a.Chain.GetHeaviestTipSet().Height()) - 1
	case eth.BlkNumPending:
		num = eth.EthUint64(a.Chain.GetHeaviestTipSet().Height())
	}

	ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(num), nil, false)
	if err != nil {
		return eth.EthBlock{}, xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	return a.newEthBlockFromFilecoinTipSet(ctx, ts, fullTxInfo)
}

func (a *EthModule) EthGetTransactionByHash(ctx context.Context, txHash *eth.EthHash) (*eth.EthTx, error) {
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

func (a *EthModule) EthGetTransactionCount(ctx context.Context, sender eth.EthAddress, blkParam string) (eth.EthUint64, error) {
	addr, err := sender.ToFilecoinAddress()
	if err != nil {
		return eth.EthUint64(0), nil
	}
	nonce, err := a.Mpool.GetNonce(ctx, addr, types.EmptyTSK)
	if err != nil {
		return eth.EthUint64(0), nil
	}
	return eth.EthUint64(nonce), nil
}

func (a *EthModule) EthGetTransactionReceipt(ctx context.Context, txHash eth.EthHash) (*api.EthTxReceipt, error) {
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

	receipt, err := NewEthTxReceipt(tx, msgLookup, replay)
	if err != nil {
		return nil, nil
	}
	return &receipt, nil
}

func (a *EthModule) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash eth.EthHash, txIndex eth.EthUint64) (eth.EthTx, error) {
	return eth.EthTx{}, nil
}

func (a *EthModule) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum eth.EthUint64, txIndex eth.EthUint64) (eth.EthTx, error) {
	return eth.EthTx{}, nil
}

// EthGetCode returns string value of the compiled bytecode
func (a *EthModule) EthGetCode(ctx context.Context, ethAddr eth.EthAddress, blkOpt string) (eth.EthBytes, error) {
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

func (a *EthModule) EthGetStorageAt(ctx context.Context, ethAddr eth.EthAddress, position eth.EthBytes, blkParam string) (eth.EthBytes, error) {
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

func (a *EthModule) EthGetBalance(ctx context.Context, address eth.EthAddress, blkParam string) (eth.EthBigInt, error) {
	filAddr, err := address.ToFilecoinAddress()
	if err != nil {
		return eth.EthBigInt{}, err
	}

	actor, err := a.StateGetActor(ctx, filAddr, types.EmptyTSK)
	if xerrors.Is(err, types.ErrActorNotFound) {
		return eth.EthBigIntZero, nil
	} else if err != nil {
		return eth.EthBigInt{}, err
	}

	return eth.EthBigInt{Int: actor.Balance.Int}, nil
}

func (a *EthModule) EthChainId(ctx context.Context) (eth.EthUint64, error) {
	return eth.EthUint64(build.Eip155ChainId), nil
}

func (a *EthModule) EthFeeHistory(ctx context.Context, blkCount eth.EthUint64, newestBlkNum string, rewardPercentiles []float64) (eth.EthFeeHistory, error) {
	if blkCount > 1024 {
		return eth.EthFeeHistory{}, fmt.Errorf("block count should be smaller than 1024")
	}

	newestBlkHeight := uint64(a.Chain.GetHeaviestTipSet().Height())

	// TODO https://github.com/filecoin-project/ref-fvm/issues/1016
	var blkNum eth.EthUint64
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
		return eth.EthFeeHistory{}, fmt.Errorf("cannot load find block height: %v", newestBlkHeight)
	}

	// FIXME: baseFeePerGas should include the next block after the newest of the returned range, because this
	// can be inferred from the newest block. we use the newest block's baseFeePerGas for now but need to fix it
	// In other words, due to deferred execution, we might not be returning the most useful value here for the client.
	baseFeeArray := []eth.EthBigInt{eth.EthBigInt(ts.Blocks()[0].ParentBaseFee)}
	gasUsedRatioArray := []float64{}

	for ts.Height() >= abi.ChainEpoch(oldestBlkHeight) {
		// Unfortunately we need to rebuild the full message view so we can
		// totalize gas used in the tipset.
		block, err := a.newEthBlockFromFilecoinTipSet(ctx, ts, false)
		if err != nil {
			return eth.EthFeeHistory{}, fmt.Errorf("cannot create eth block: %v", err)
		}

		// both arrays should be reversed at the end
		baseFeeArray = append(baseFeeArray, eth.EthBigInt(ts.Blocks()[0].ParentBaseFee))
		gasUsedRatioArray = append(gasUsedRatioArray, float64(block.GasUsed)/float64(build.BlockGasLimit))

		parentTsKey := ts.Parents()
		ts, err = a.Chain.LoadTipSet(ctx, parentTsKey)
		if err != nil {
			return eth.EthFeeHistory{}, fmt.Errorf("cannot load tipset key: %v", parentTsKey)
		}
	}

	// Reverse the arrays; we collected them newest to oldest; the client expects oldest to newest.

	for i, j := 0, len(baseFeeArray)-1; i < j; i, j = i+1, j-1 {
		baseFeeArray[i], baseFeeArray[j] = baseFeeArray[j], baseFeeArray[i]
	}
	for i, j := 0, len(gasUsedRatioArray)-1; i < j; i, j = i+1, j-1 {
		gasUsedRatioArray[i], gasUsedRatioArray[j] = gasUsedRatioArray[j], gasUsedRatioArray[i]
	}

	return eth.EthFeeHistory{
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

func (a *EthModule) EthProtocolVersion(ctx context.Context) (eth.EthUint64, error) {
	height := a.Chain.GetHeaviestTipSet().Height()
	return eth.EthUint64(a.StateManager.GetNetworkVersion(ctx, height)), nil
}

func (a *EthModule) EthMaxPriorityFeePerGas(ctx context.Context) (eth.EthBigInt, error) {
	gasPremium, err := a.GasAPI.GasEstimateGasPremium(ctx, 0, builtin.SystemActorAddr, 10000, types.EmptyTSK)
	if err != nil {
		return eth.EthBigInt(big.Zero()), err
	}
	return eth.EthBigInt(gasPremium), nil
}

func (a *EthModule) EthGasPrice(ctx context.Context) (eth.EthBigInt, error) {
	// According to Geth's implementation, eth_gasPrice should return base + tip
	// Ref: https://github.com/ethereum/pm/issues/328#issuecomment-853234014

	ts := a.Chain.GetHeaviestTipSet()
	baseFee := ts.Blocks()[0].ParentBaseFee

	premium, err := a.EthMaxPriorityFeePerGas(ctx)
	if err != nil {
		return eth.EthBigInt(big.Zero()), nil
	}

	gasPrice := big.Add(baseFee, big.Int(premium))
	return eth.EthBigInt(gasPrice), nil
}

func (a *EthModule) EthSendRawTransaction(ctx context.Context, rawTx eth.EthBytes) (eth.EthHash, error) {
	txArgs, err := eth.ParseEthTxArgs(rawTx)
	if err != nil {
		return eth.EmptyEthHash, err
	}

	smsg, err := txArgs.ToSignedMessage()
	if err != nil {
		return eth.EmptyEthHash, err
	}

	_, err = a.StateAPI.StateGetActor(ctx, smsg.Message.To, types.EmptyTSK)
	if err != nil {
		// if actor does not exist on chain yet, set the method to 0 because
		// embryos only implement method 0
		smsg.Message.Method = builtin.MethodSend
	}

	cid, err := a.MpoolAPI.MpoolPush(ctx, smsg)
	if err != nil {
		return eth.EmptyEthHash, err
	}
	return eth.NewEthHashFromCid(cid)
}

func (a *EthModule) ethCallToFilecoinMessage(ctx context.Context, tx eth.EthCall) (*types.Message, error) {
	var err error
	var from address.Address
	if tx.From == nil {
		// Send from the filecoin "system" address.
		from, err = (eth.EthAddress{}).ToFilecoinAddress()
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

func (a *EthModule) EthEstimateGas(ctx context.Context, tx eth.EthCall) (eth.EthUint64, error) {
	msg, err := a.ethCallToFilecoinMessage(ctx, tx)
	if err != nil {
		return eth.EthUint64(0), err
	}

	// Set the gas limit to the zero sentinel value, which makes
	// gas estimation actually run.
	msg.GasLimit = 0

	msg, err = a.GasAPI.GasEstimateMessageGas(ctx, msg, nil, types.EmptyTSK)
	if err != nil {
		return eth.EthUint64(0), err
	}

	return eth.EthUint64(msg.GasLimit), nil
}

func (a *EthModule) EthCall(ctx context.Context, tx eth.EthCall, blkParam string) (eth.EthBytes, error) {
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
	return eth.EthBytes{}, nil
}

func (a *EthModule) newEthBlockFromFilecoinTipSet(ctx context.Context, ts *types.TipSet, fullTxInfo bool) (eth.EthBlock, error) {
	parent, err := a.Chain.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return eth.EthBlock{}, err
	}
	parentKeyCid, err := parent.Key().Cid()
	if err != nil {
		return eth.EthBlock{}, err
	}
	parentBlkHash, err := eth.NewEthHashFromCid(parentKeyCid)
	if err != nil {
		return eth.EthBlock{}, err
	}

	blkCid, err := ts.Key().Cid()
	if err != nil {
		return eth.EthBlock{}, err
	}
	blkHash, err := eth.NewEthHashFromCid(blkCid)
	if err != nil {
		return eth.EthBlock{}, err
	}

	blkMsgs, err := a.Chain.BlockMsgsForTipset(ctx, ts)
	if err != nil {
		return eth.EthBlock{}, xerrors.Errorf("error loading messages for tipset: %v: %w", ts, err)
	}

	block := eth.NewEthBlock()

	// this seems to be a very expensive way to get gasUsed of the block. may need to find an efficient way to do it
	gasUsed := int64(0)
	for _, blkMsg := range blkMsgs {
		for _, msg := range append(blkMsg.BlsMessages, blkMsg.SecpkMessages...) {
			msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, msg.Cid(), api.LookbackNoLimit, true)
			if err != nil || msgLookup == nil {
				return eth.EthBlock{}, nil
			}
			gasUsed += msgLookup.Receipt.GasUsed

			if fullTxInfo {
				tx, err := a.newEthTxFromFilecoinMessageLookup(ctx, msgLookup)
				if err != nil {
					return eth.EthBlock{}, nil
				}
				block.Transactions = append(block.Transactions, tx)
			} else {
				hash, err := eth.NewEthHashFromCid(msg.Cid())
				if err != nil {
					return eth.EthBlock{}, err
				}
				block.Transactions = append(block.Transactions, hash.String())
			}
		}
	}

	block.Hash = blkHash
	block.Number = eth.EthUint64(ts.Height())
	block.ParentHash = parentBlkHash
	block.Timestamp = eth.EthUint64(ts.Blocks()[0].Timestamp)
	block.BaseFeePerGas = eth.EthBigInt{Int: ts.Blocks()[0].ParentBaseFee.Int}
	block.GasUsed = eth.EthUint64(gasUsed)
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
func (a *EthModule) lookupEthAddress(ctx context.Context, addr address.Address) (eth.EthAddress, error) {
	// Attempt to convert directly.
	if ethAddr, ok, err := eth.TryEthAddressFromFilecoinAddress(addr, false); err != nil {
		return eth.EthAddress{}, err
	} else if ok {
		return ethAddr, nil
	}

	// Lookup on the target actor.
	actor, err := a.StateAPI.StateGetActor(ctx, addr, types.EmptyTSK)
	if err != nil {
		return eth.EthAddress{}, err
	}
	if actor.Address != nil {
		if ethAddr, ok, err := eth.TryEthAddressFromFilecoinAddress(*actor.Address, false); err != nil {
			return eth.EthAddress{}, err
		} else if ok {
			return ethAddr, nil
		}
	}

	// Check if we already have an ID addr, and use it if possible.
	if ethAddr, ok, err := eth.TryEthAddressFromFilecoinAddress(addr, true); err != nil {
		return eth.EthAddress{}, err
	} else if ok {
		return ethAddr, nil
	}

	// Otherwise, resolve the ID addr.
	idAddr, err := a.StateAPI.StateLookupID(ctx, addr, types.EmptyTSK)
	if err != nil {
		return eth.EthAddress{}, err
	}
	return eth.EthAddressFromFilecoinAddress(idAddr)
}

func (a *EthModule) newEthTxFromFilecoinMessageLookup(ctx context.Context, msgLookup *api.MsgLookup) (eth.EthTx, error) {
	if msgLookup == nil {
		return eth.EthTx{}, fmt.Errorf("msg does not exist")
	}
	cid := msgLookup.Message
	txHash, err := eth.NewEthHashFromCid(cid)
	if err != nil {
		return eth.EthTx{}, err
	}

	ts, err := a.Chain.LoadTipSet(ctx, msgLookup.TipSet)
	if err != nil {
		return eth.EthTx{}, err
	}

	// This tx is located in the parent tipset
	parentTs, err := a.Chain.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return eth.EthTx{}, err
	}

	parentTsCid, err := parentTs.Key().Cid()
	if err != nil {
		return eth.EthTx{}, err
	}

	// lookup the transactionIndex
	txIdx := -1
	msgs, err := a.Chain.MessagesForTipset(ctx, parentTs)
	if err != nil {
		return eth.EthTx{}, err
	}
	for i, msg := range msgs {
		if msg.Cid() == msgLookup.Message {
			txIdx = i
		}
	}
	if txIdx == -1 {
		return eth.EthTx{}, fmt.Errorf("cannot find the msg in the tipset")
	}

	blkHash, err := eth.NewEthHashFromCid(parentTsCid)
	if err != nil {
		return eth.EthTx{}, err
	}

	msg, err := a.ChainAPI.ChainGetMessage(ctx, msgLookup.Message)
	if err != nil {
		return eth.EthTx{}, err
	}

	fromEthAddr, err := a.lookupEthAddress(ctx, msg.From)
	if err != nil {
		return eth.EthTx{}, err
	}

	toEthAddr, err := a.lookupEthAddress(ctx, msg.To)
	if err != nil {
		return eth.EthTx{}, err
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
			return eth.EthTx{}, err
		}
	}
	// Otherwise, try to decode as a cbor byte array.
	// TODO: Actually check if this is an ethereum call. This code will work for demo purposes, but is not correct.
	if toAddr != nil {
		if decodedParams, err := cbg.ReadByteArray(bytes.NewReader(msg.Params), uint64(len(msg.Params))); err == nil {
			input = decodedParams
		}
	}

	tx := eth.EthTx{
		ChainID:              eth.EthUint64(build.Eip155ChainId),
		Hash:                 txHash,
		BlockHash:            blkHash,
		BlockNumber:          eth.EthUint64(parentTs.Height()),
		From:                 fromEthAddr,
		To:                   toAddr,
		Value:                eth.EthBigInt(msg.Value),
		Type:                 eth.EthUint64(2),
		TransactionIndex:     eth.EthUint64(txIdx),
		Gas:                  eth.EthUint64(msg.GasLimit),
		MaxFeePerGas:         eth.EthBigInt(msg.GasFeeCap),
		MaxPriorityFeePerGas: eth.EthBigInt(msg.GasPremium),
		V:                    eth.EthBytes{},
		R:                    eth.EthBytes{},
		S:                    eth.EthBytes{},
		Input:                input,
	}
	return tx, nil
}

func NewEthTxReceipt(tx eth.EthTx, lookup *api.MsgLookup, replay *api.InvocResult) (api.EthTxReceipt, error) {
	receipt := api.EthTxReceipt{
		TransactionHash:  tx.Hash,
		TransactionIndex: tx.TransactionIndex,
		BlockHash:        tx.BlockHash,
		BlockNumber:      tx.BlockNumber,
		From:             tx.From,
		To:               tx.To,
		StateRoot:        eth.EmptyEthHash,
		LogsBloom:        []byte{0},
		Logs:             []string{},
	}

	if receipt.To == nil && lookup.Receipt.ExitCode.IsSuccess() {
		// Create and Create2 return the same things.
		var ret eam.CreateReturn
		if err := ret.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)); err != nil {
			return api.EthTxReceipt{}, xerrors.Errorf("failed to parse contract creation result: %w", err)
		}
		addr := eth.EthAddress(ret.EthAddress)
		receipt.ContractAddress = &addr
	}

	if lookup.Receipt.ExitCode.IsSuccess() {
		receipt.Status = 1
	}
	if lookup.Receipt.ExitCode.IsError() {
		receipt.Status = 0
	}

	receipt.GasUsed = eth.EthUint64(lookup.Receipt.GasUsed)

	// TODO: handle CumulativeGasUsed
	receipt.CumulativeGasUsed = eth.EmptyEthInt

	effectiveGasPrice := big.Div(replay.GasCost.TotalCost, big.NewInt(lookup.Receipt.GasUsed))
	receipt.EffectiveGasPrice = eth.EthBigInt(effectiveGasPrice)
	return receipt, nil
}
