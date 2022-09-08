package full

import (
	"context"
	"strconv"
	"strings"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

type EthModuleAPI interface {
	EthBlockNumber(context.Context) (api.EthInt, error)
	EthAccounts(context.Context) ([]api.EthAddress, error)
	EthGetBlockTransactionCountByNumber(context.Context, string) (api.EthInt, error)
	EthGetBlockTransactionCountByHash(context.Context, string) (api.EthInt, error)
	EthGetBlockByHash(ctx context.Context, blkHash string, fullTxInfo bool) (api.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNumHex string, fullTxInfo bool) (api.EthBlock, error)
	EthGetTransactionByHash(ctx context.Context, txHash string) (api.EthTx, error)
	EthGetTransactionCount(ctx context.Context, sender string, blkOpt string) (api.EthInt, error)
	EthGetTransactionReceipt(ctx context.Context, blkHash string) (api.EthTxReceipt, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash string, txIndexHex string) (api.EthTx, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNumHex string, txIndexHex string) (api.EthTx, error)
	EthGetCode(ctx context.Context, address string) (string, error)
	EthGetStorageAt(ctx context.Context, address string, positionHex string, blkParam string) (string, error)
	EthGetBalance(ctx context.Context, address string, blkParam string) (api.EthBigInt, error)
	EthChainId(ctx context.Context) (api.EthInt, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (api.EthInt, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (api.EthInt, error)
	EthGasPrice(ctx context.Context) (api.EthInt, error)
	EthSendRawTransaction(ctx context.Context) (api.EthHash, error)
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

func (a *EthModule) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNumHex string) (api.EthInt, error) {
	blkNum, err := strconv.ParseInt(strings.Replace(blkNumHex, "0x", "", -1), 16, 64)
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("invalid block number %s: %w", blkNumHex, err)
	}

	ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(blkNum), nil, false)
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}

	count, err := a.countTipsetMsgs(ctx, ts)
	return api.EthInt(count), err
}

func (a *EthModule) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash string) (api.EthInt, error) {
	hash, err := api.EthHashFromHex(blkHash)
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("invalid hash %s: %w", blkHash, err)
	}

	ts, err := a.Chain.GetTipSetByCid(ctx, hash.ToCid())
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}
	count, err := a.countTipsetMsgs(ctx, ts)
	return api.EthInt(count), err
}

func (a *EthModule) EthGetBlockByHash(ctx context.Context, blkHash string, fullTxInfo bool) (api.EthBlock, error) {
	return api.EthBlock{}, nil
}

func (a *EthModule) EthGetBlockByNumber(ctx context.Context, blkNumHex string, fullTxInfo bool) (api.EthBlock, error) {
	return api.EthBlock{}, nil
}

func (a *EthModule) EthGetTransactionByHash(ctx context.Context, txHash string) (api.EthTx, error) {
	return api.EthTx{}, nil
}

func (a *EthModule) EthGetTransactionCount(ctx context.Context, sender string, blkParam string) (api.EthInt, error) {
	return api.EthInt(0), nil
}

func (a *EthModule) EthGetTransactionReceipt(ctx context.Context, blkHash string) (api.EthTxReceipt, error) {
	return api.EthTxReceipt{}, nil
}

func (a *EthModule) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash string, txIndexHex string) (api.EthTx, error) {
	return api.EthTx{}, nil
}

func (a *EthModule) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNumHex string, txIndexHex string) (api.EthTx, error) {
	return api.EthTx{}, nil
}

// EthGetCode returns string value of the compiled bytecode
func (a *EthModule) EthGetCode(ctx context.Context, address string) (string, error) {
	return "", nil
}

func (a *EthModule) EthGetStorageAt(ctx context.Context, address string, positionHex string, blkParam string) (string, error) {
	return "", nil
}

func (a *EthModule) EthGetBalance(ctx context.Context, address string, blkParam string) (api.EthBigInt, error) {
	addr, err := api.EthAddressFromHex(address)
	if err != nil {
		return api.EthBigInt{}, xerrors.Errorf("cannot parse address: %w", err)
	}

	filAddr, err := addr.ToFilecoinAddress()
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
	return "1", nil
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

func (a *EthModule) EthSendRawTransaction(ctx context.Context) (api.EthHash, error) {
	return api.EthHash{}, nil
}

// func (a *EthModule) EthEstimateGas(ctx context.Context, tx api.EthTx, blkParam string) (api.EthInt, error) {
// 	return api.EthInt(0), nil
// }
//
// func (a *EthModule) EthCall(ctx context.Context, tx api.EthTx, blkParam string) (string, error) {
// 	return "", nil
// }
//
