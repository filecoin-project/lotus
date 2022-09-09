package full

import (
	"context"
	"strconv"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
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
	EthGetTransactionReceipt(ctx context.Context, blkHash api.EthHash) (api.EthTxReceipt, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash api.EthHash, txIndex api.EthInt) (api.EthTx, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum api.EthInt, txIndex api.EthInt) (api.EthTx, error)
	EthGetCode(ctx context.Context, address api.EthAddress) (string, error)
	EthGetStorageAt(ctx context.Context, address api.EthAddress, position api.EthInt, blkParam string) (string, error)
	EthGetBalance(ctx context.Context, address api.EthAddress, blkParam string) (api.EthBigInt, error)
	EthChainId(ctx context.Context) (api.EthInt, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (api.EthInt, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (api.EthInt, error)
	EthGasPrice(ctx context.Context) (api.EthInt, error)
	// EthSendRawTransaction(ctx context.Context, tx api.EthTx) (api.EthHash, error)
}

var _ EthModuleAPI = *new(api.FullNode)

// EthModule provides a default implementation of EthModuleAPI.
// It can be swapped out with another implementation through Dependency
// Injection (for example with a thin RPC client).
type EthModule struct {
	fx.In

	Chain *store.ChainStore
	StateAPI
}

var _ EthModuleAPI = (*EthModule)(nil)

type EthAPI struct {
	fx.In

	Chain *store.ChainStore

	EthModuleAPI
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
	return api.EthBlock{}, nil
}

func (a *EthModule) EthGetBlockByNumber(ctx context.Context, blkNum api.EthInt, fullTxInfo bool) (api.EthBlock, error) {
	return api.EthBlock{}, nil
}

func (a *EthModule) EthGetTransactionByHash(ctx context.Context, txHash api.EthHash) (api.EthTx, error) {
	return api.EthTx{}, nil
}

func (a *EthModule) EthGetTransactionCount(ctx context.Context, sender api.EthAddress, blkParam string) (api.EthInt, error) {
	return api.EthInt(0), nil
}

func (a *EthModule) EthGetTransactionReceipt(ctx context.Context, blkHash api.EthHash) (api.EthTxReceipt, error) {
	return api.EthTxReceipt{}, nil
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
	return api.EthInt(0), nil
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
	return api.EthInt(0), nil
}

func (a *EthModule) EthMaxPriorityFeePerGas(ctx context.Context) (api.EthInt, error) {
	return api.EthInt(0), nil
}

func (a *EthModule) EthGasPrice(ctx context.Context) (api.EthInt, error) {
	return api.EthInt(0), nil
}

// func (a *EthModule) EthSendRawTransaction(ctx context.Context tx api.EthTx) (api.EthHash, error) {
// 	return api.EthHash{}, nil
// }

// func (a *EthModule) EthEstimateGas(ctx context.Context, tx api.EthTx, blkParam string) (api.EthInt, error) {
// 	return api.EthInt(0), nil
// }
//
// func (a *EthModule) EthCall(ctx context.Context, tx api.EthTx, blkParam string) (string, error) {
// 	return "", nil
// }
//
