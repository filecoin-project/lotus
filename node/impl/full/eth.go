package full

import (
	"context"
	"strconv"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type EthModuleAPI interface {
	EthBlockNumber(ctx context.Context) (api.EthInt, error)
	EthAccounts(ctx context.Context) ([]api.EthAddress, error)
	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum api.EthInt) (api.EthInt, error)
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash api.EthHash) (api.EthInt, error)
	EthGetBlockByHash(ctx context.Context, blkHash api.EthHash, fullTxInfo bool) (api.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNum api.EthInt, fullTxInfo bool) (api.EthBlock, error)
	EthGetTransactionByHash(ctx context.Context, txHash api.EthHash) (api.EthTx, error)
	EthGetTransactionCount(ctx context.Context, sender api.EthAddress, blkOpt string) (api.EthInt, error)
	EthGetTransactionReceipt(ctx context.Context, txHash api.EthHash) (api.EthTxReceipt, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash api.EthHash, txIndex api.EthInt) (api.EthTx, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum api.EthInt, txIndex api.EthInt) (api.EthTx, error)
	EthGetCode(ctx context.Context, address api.EthAddress) (string, error)
	EthGetStorageAt(ctx context.Context, address api.EthAddress, position api.EthInt, blkParam string) (string, error)
	EthGetBalance(ctx context.Context, address api.EthAddress, blkParam string) (api.EthBigInt, error)
	EthChainId(ctx context.Context) (api.EthInt, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (api.EthInt, error)
	EthGasPrice(ctx context.Context) (api.EthBigInt, error)
	EthEstimateGas(ctx context.Context, tx api.EthCall, blkParam string) (api.EthInt, error)
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

func (a *EthModule) EthBlockNumber(context.Context) (api.EthInt, error) {
	height := a.Chain.GetHeaviestTipSet().Height()
	return api.EthInt(height), nil
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

func (a *EthModule) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum api.EthInt) (api.EthInt, error) {
	ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(blkNum), nil, false)
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}

	count, err := a.countTipsetMsgs(ctx, ts)
	return api.EthInt(count), err
}

func (a *EthModule) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash api.EthHash) (api.EthInt, error) {
	ts, err := a.Chain.GetTipSetByCid(ctx, blkHash.ToCid())
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	count, err := a.countTipsetMsgs(ctx, ts)
	return api.EthInt(count), err
}

func (a *EthModule) EthGetBlockByHash(ctx context.Context, blkHash api.EthHash, fullTxInfo bool) (api.EthBlock, error) {
	ts, err := a.Chain.GetTipSetByCid(ctx, blkHash.ToCid())
	if err != nil {
		return api.EthBlock{}, xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	return a.ethBlockFromFilecoinTipSet(ctx, ts, fullTxInfo)
}

func (a *EthModule) EthGetBlockByNumber(ctx context.Context, blkNum api.EthInt, fullTxInfo bool) (api.EthBlock, error) {
	ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(blkNum), nil, false)
	if err != nil {
		return api.EthBlock{}, xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	return a.ethBlockFromFilecoinTipSet(ctx, ts, fullTxInfo)
}

func (a *EthModule) EthGetTransactionByHash(ctx context.Context, txHash api.EthHash) (api.EthTx, error) {
	cid := txHash.ToCid()

	msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, cid, api.LookbackNoLimit, true)
	if err != nil {
		return api.EthTx{}, nil
	}

	tx, err := a.ethTxFromFilecoinMessageLookup(ctx, msgLookup)
	if err != nil {
		return api.EthTx{}, err
	}
	return tx, nil
}

func (a *EthModule) EthGetTransactionCount(ctx context.Context, sender api.EthAddress, blkParam string) (api.EthInt, error) {
	addr, err := sender.ToFilecoinAddress()
	if err != nil {
		return api.EthInt(0), err
	}
	nonce, err := a.Mpool.GetNonce(ctx, addr, types.EmptyTSK)
	if err != nil {
		return api.EthInt(0), err
	}
	return api.EthInt(nonce), nil
}

func (a *EthModule) EthGetTransactionReceipt(ctx context.Context, txHash api.EthHash) (api.EthTxReceipt, error) {
	cid := txHash.ToCid()

	msgLookup, err := a.StateAPI.StateSearchMsg(ctx, types.EmptyTSK, cid, api.LookbackNoLimit, true)
	if err != nil {
		return api.EthTxReceipt{}, err
	}

	tx, err := a.ethTxFromFilecoinMessageLookup(ctx, msgLookup)
	if err != nil {
		return api.EthTxReceipt{}, err
	}

	replay, err := a.StateAPI.StateReplay(ctx, types.EmptyTSK, cid)
	if err != nil {
		return api.EthTxReceipt{}, err
	}

	receipt := api.NewEthTxReceipt(tx)
	err = receipt.PopulateReceipt(msgLookup, replay)
	if err != nil {
		return api.EthTxReceipt{}, err
	}
	return receipt, nil
}

func (a *EthModule) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash api.EthHash, txIndex api.EthInt) (api.EthTx, error) {
	return api.EthTx{}, nil
}

func (a *EthModule) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum api.EthInt, txIndex api.EthInt) (api.EthTx, error) {
	return api.EthTx{}, nil
}

// EthGetCode returns string value of the compiled bytecode
func (a *EthModule) EthGetCode(ctx context.Context, address api.EthAddress) (string, error) {
	return "", nil
}

func (a *EthModule) EthGetStorageAt(ctx context.Context, address api.EthAddress, position api.EthInt, blkParam string) (string, error) {
	return "", nil
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

func (a *EthModule) EthChainId(ctx context.Context) (api.EthInt, error) {
	return api.EthInt(build.Eip155ChainId), nil
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

func (a *EthModule) EthProtocolVersion(ctx context.Context) (api.EthInt, error) {
	height := a.Chain.GetHeaviestTipSet().Height()
	return api.EthInt(a.StateManager.GetNetworkVersion(ctx, height)), nil
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
	return api.EthHash{}, nil
}

func (a *EthModule) applyEvmMsg(ctx context.Context, tx api.EthCall) (*api.InvocResult, error) {
	from, err := tx.From.ToFilecoinAddress()
	if err != nil {
		return nil, err
	}
	to, err := tx.To.ToFilecoinAddress()
	if err != nil {
		return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
	}
	msg := &types.Message{
		From:       from,
		To:         to,
		Value:      big.Int(tx.Value),
		Method:     abi.MethodNum(2),
		Params:     tx.Data,
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
	if res.MsgRct.ExitCode != exitcode.Ok {
		return nil, xerrors.Errorf("message execution failed: exit %s, reason: %s", res.MsgRct.ExitCode, res.Error)
	}
	return res, nil
}

func (a *EthModule) EthEstimateGas(ctx context.Context, tx api.EthCall, blkParam string) (api.EthInt, error) {
	invokeResult, err := a.applyEvmMsg(ctx, tx)
	if err != nil {
		return api.EthInt(0), err
	}
	ret := invokeResult.MsgRct.GasUsed
	return api.EthInt(ret), nil
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

	block.Number = api.EthInt(ts.Height())
	block.ParentHash = parentBlkHash
	block.Timestamp = api.EthInt(ts.Blocks()[0].Timestamp)
	block.BaseFeePerGas = api.EthBigInt{Int: ts.Blocks()[0].ParentBaseFee.Int}
	block.GasUsed = api.EthInt(gasUsed)
	return block, nil
}

func (a *EthModule) ethTxFromFilecoinMessageLookup(ctx context.Context, msgLookup *api.MsgLookup) (api.EthTx, error) {
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

	tx := api.EthTx{
		ChainID:              api.EthInt(build.Eip155ChainId),
		Hash:                 txHash,
		BlockHash:            blkHash,
		BlockNumber:          api.EthInt(msgLookup.Height),
		From:                 fromEthAddr,
		To:                   toEthAddr,
		Value:                api.EthBigInt(msg.Value),
		Type:                 api.EthInt(2),
		Gas:                  api.EthInt(msg.GasLimit),
		MaxFeePerGas:         api.EthBigInt(msg.GasFeeCap),
		MaxPriorityFeePerGas: api.EthBigInt(msg.GasPremium),
		V:                    api.EthBigIntZero,
		R:                    api.EthBigIntZero,
		S:                    api.EthBigIntZero,
		// TODO: Input:
	}
	return tx, nil
}
