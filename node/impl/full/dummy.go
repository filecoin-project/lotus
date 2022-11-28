package full

import (
	"context"
	"errors"

	"github.com/filecoin-project/lotus/api"
)

var ErrImplementMe = errors.New("Not implemented yet")

type EthModuleDummy struct{}

func (e *EthModuleDummy) EthBlockNumber(ctx context.Context) (api.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthAccounts(ctx context.Context) ([]api.EthAddress, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum api.EthUint64) (api.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash api.EthHash) (api.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockByHash(ctx context.Context, blkHash api.EthHash, fullTxInfo bool) (api.EthBlock, error) {
	return api.EthBlock{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (api.EthBlock, error) {
	return api.EthBlock{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByHash(ctx context.Context, txHash *api.EthHash) (*api.EthTx, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionCount(ctx context.Context, sender api.EthAddress, blkOpt string) (api.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionReceipt(ctx context.Context, txHash api.EthHash) (*api.EthTxReceipt, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash api.EthHash, txIndex api.EthUint64) (api.EthTx, error) {
	return api.EthTx{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum api.EthUint64, txIndex api.EthUint64) (api.EthTx, error) {
	return api.EthTx{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetCode(ctx context.Context, address api.EthAddress, blkOpt string) (api.EthBytes, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetStorageAt(ctx context.Context, address api.EthAddress, position api.EthBytes, blkParam string) (api.EthBytes, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBalance(ctx context.Context, address api.EthAddress, blkParam string) (api.EthBigInt, error) {
	return api.EthBigIntZero, ErrImplementMe
}

func (e *EthModuleDummy) EthFeeHistory(ctx context.Context, blkCount api.EthUint64, newestBlk string, rewardPercentiles []float64) (api.EthFeeHistory, error) {
	return api.EthFeeHistory{}, ErrImplementMe
}

func (e *EthModuleDummy) EthChainId(ctx context.Context) (api.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) NetVersion(ctx context.Context) (string, error) {
	return "", ErrImplementMe
}

func (e *EthModuleDummy) NetListening(ctx context.Context) (bool, error) {
	return false, ErrImplementMe
}

func (e *EthModuleDummy) EthProtocolVersion(ctx context.Context) (api.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGasPrice(ctx context.Context) (api.EthBigInt, error) {
	return api.EthBigIntZero, ErrImplementMe
}

func (e *EthModuleDummy) EthEstimateGas(ctx context.Context, tx api.EthCall) (api.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthCall(ctx context.Context, tx api.EthCall, blkParam string) (api.EthBytes, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthMaxPriorityFeePerGas(ctx context.Context) (api.EthBigInt, error) {
	return api.EthBigIntZero, ErrImplementMe
}

func (e *EthModuleDummy) EthSendRawTransaction(ctx context.Context, rawTx api.EthBytes) (api.EthHash, error) {
	return api.EthHash{}, ErrImplementMe
}
