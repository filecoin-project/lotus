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
	"github.com/filecoin-project/go-state-types/builtin/v8/evm"
	init8 "github.com/filecoin-project/go-state-types/builtin/v8/init"
	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
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
	EthGetCode(ctx context.Context, address api.EthAddress) (api.EthBytes, error)
	EthGetStorageAt(ctx context.Context, address api.EthAddress, position api.EthBytes, blkParam string) (api.EthBytes, error)
	EthGetBalance(ctx context.Context, address api.EthAddress, blkParam string) (api.EthBigInt, error)
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
		return api.EthUint64(0), err
	}
	nonce, err := a.Mpool.GetNonce(ctx, addr, types.EmptyTSK)
	if err != nil {
		return api.EthUint64(0), err
	}
	return api.EthUint64(nonce), nil
}

func (a *EthModule) EthGetTransactionReceipt(ctx context.Context, txHash api.EthHash) (*api.EthTxReceipt, error) {
	cid := txHash.ToCid()

	msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, cid, api.LookbackNoLimit, true)
	if err != nil {
		return nil, err
	}

	tx, err := a.ethTxFromFilecoinMessageLookup(ctx, msgLookup)
	if err != nil {
		return nil, err
	}

	replay, err := a.StateAPI.StateReplay(ctx, types.EmptyTSK, cid)
	if err != nil {
		return nil, err
	}

	receipt, err := api.NewEthTxReceipt(tx, msgLookup, replay)
	if err != nil {
		return nil, err
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
func (a *EthModule) EthGetCode(ctx context.Context, ethAddr api.EthAddress) (api.EthBytes, error) {
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
		Method:     abi.MethodNum(3), // GetBytecode
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
		return nil, xerrors.Errorf("Call failed: %w", err)
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
		Method:     abi.MethodNum(4), // GetStorageAt
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
	if err != nil {
		return api.EthBigInt{}, err
	}

	return api.EthBigInt{Int: actor.Balance.Int}, nil
}

func (a *EthModule) EthChainId(ctx context.Context) (api.EthUint64, error) {
	return api.EthUint64(build.Eip155ChainId), nil
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

func (a *EthModule) applyEvmMsg(ctx context.Context, tx api.EthCall) (*api.InvocResult, error) {
	// FIXME: this is a workaround, remove this when f4 address is ready
	var from address.Address
	var err error
	if tx.From[0] == 0xff && tx.From[1] == 0 && tx.From[2] == 0 {
		addr, err := tx.From.ToFilecoinAddress()
		if err != nil {
			return nil, err
		}
		from = addr
	} else {
		id := uint64(100)
		for ; id < 300; id++ {
			idAddr, err := address.NewIDAddress(id)
			if err != nil {
				return nil, err
			}
			from = idAddr
			act, err := a.StateGetActor(ctx, idAddr, types.EmptyTSK)
			if err != nil {
				return nil, err
			}
			if builtinactors.IsAccountActor(act.Code) {
				break
			}
		}
		if id == 300 {
			return nil, fmt.Errorf("cannot find a dummy account")
		}
	}

	var params []byte
	var to address.Address
	if tx.To == nil {
		to = builtintypes.InitActorAddr
		constructorParams, err := actors.SerializeParams(&evm.ConstructorParams{
			Bytecode:  tx.Data,
			InputData: []byte{},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to serialize constructor params: %w", err)
		}

		evmActorCid, ok := actors.GetActorCodeID(actors.Version8, "evm")
		if !ok {
			return nil, fmt.Errorf("failed to lookup evm actor code CID")
		}

		params, err = actors.SerializeParams(&init8.ExecParams{
			CodeCID:           evmActorCid,
			ConstructorParams: constructorParams,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to serialize init actor exec params: %w", err)
		}
	} else {
		addr, err := tx.To.ToFilecoinAddress()
		if err != nil {
			return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
		}
		to = addr
		params = tx.Data
	}

	msg := &types.Message{
		From:       from,
		To:         to,
		Value:      big.Int(tx.Value),
		Method:     abi.MethodNum(2),
		Params:     params,
		GasLimit:   build.BlockGasLimit,
		GasFeeCap:  big.Zero(),
		GasPremium: big.Zero(),
	}
	ts := a.Chain.GetHeaviestTipSet()

	// Try calling until we find a height with no migration.
	var res *api.InvocResult
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
	invokeResult, err := a.applyEvmMsg(ctx, tx)
	if err != nil {
		return api.EthUint64(0), err
	}
	ret := invokeResult.MsgRct.GasUsed
	return api.EthUint64(ret), nil
}

func (a *EthModule) EthCall(ctx context.Context, tx api.EthCall, blkParam string) (api.EthBytes, error) {
	invokeResult, err := a.applyEvmMsg(ctx, tx)
	if err != nil {
		return nil, err
	}
	if len(invokeResult.MsgRct.Return) > 0 {
		return api.EthBytes(invokeResult.MsgRct.Return), nil
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

func (a *EthModule) ethTxFromFilecoinMessageLookup(ctx context.Context, msgLookup *api.MsgLookup) (api.EthTx, error) {
	if msgLookup == nil {
		return api.EthTx{}, fmt.Errorf("msg does not exist")
	}
	cid := msgLookup.Message
	txHash, err := api.EthHashFromCid(cid)
	if err != nil {
		return api.EthTx{}, err
	}

	tsCid, err := msgLookup.TipSet.Cid()
	if err != nil {
		return api.EthTx{}, err
	}

	blkHash, err := api.EthHashFromCid(tsCid)
	if err != nil {
		return api.EthTx{}, err
	}

	msg, err := a.ChainAPI.ChainGetMessage(ctx, msgLookup.Message)
	if err != nil {
		return api.EthTx{}, err
	}

	fromFilIdAddr, err := a.StateAPI.StateLookupID(ctx, msg.From, types.EmptyTSK)
	if err != nil {
		return api.EthTx{}, err
	}

	fromEthAddr, err := api.EthAddressFromFilecoinIDAddress(fromFilIdAddr)
	if err != nil {
		return api.EthTx{}, err
	}

	toFilAddr, err := a.StateAPI.StateLookupID(ctx, msg.To, types.EmptyTSK)
	if err != nil {
		return api.EthTx{}, err
	}

	toEthAddr, err := api.EthAddressFromFilecoinIDAddress(toFilAddr)
	if err != nil {
		return api.EthTx{}, err
	}

	toAddr := &toEthAddr
	_, err = api.CheckContractCreation(msgLookup)
	if err == nil {
		toAddr = nil
	}

	tx := api.EthTx{
		ChainID:              api.EthUint64(build.Eip155ChainId),
		Hash:                 txHash,
		BlockHash:            blkHash,
		BlockNumber:          api.EthUint64(msgLookup.Height),
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
		Input:                msg.Params,
	}
	return tx, nil
}
