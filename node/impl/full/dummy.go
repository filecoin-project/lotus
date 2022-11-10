package full

import (
	"context"
	"errors"

	"github.com/filecoin-project/lotus/api"
)

var ImplementMe = errors.New("Not implemented yet")

type EthModuleDummy struct{}

func (e *EthModuleDummy) EthBlockNumber(ctx context.Context) (api.EthUint64, error) {
	return 0, ImplementMe
}

func (e *EthModuleDummy) EthAccounts(ctx context.Context) ([]api.EthAddress, error) {
	return nil, ImplementMe
}

func (e *EthModuleDummy) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum api.EthUint64) (api.EthUint64, error) {
	return 0, ImplementMe
}

func (e *EthModuleDummy) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash api.EthHash) (api.EthUint64, error) {
	return 0, ImplementMe
}

func (e *EthModuleDummy) EthGetBlockByHash(ctx context.Context, blkHash api.EthHash, fullTxInfo bool) (api.EthBlock, error) {
	return api.EthBlock{}, ImplementMe
}

func (e *EthModuleDummy) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (api.EthBlock, error) {
	return api.EthBlock{}, ImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByHash(ctx context.Context, txHash *api.EthHash) (*api.EthTx, error) {
	return nil, ImplementMe
}

func (e *EthModuleDummy) EthGetTransactionCount(ctx context.Context, sender api.EthAddress, blkOpt string) (api.EthUint64, error) {
	return 0, ImplementMe
}

func (e *EthModuleDummy) EthGetTransactionReceipt(ctx context.Context, txHash api.EthHash) (*api.EthTxReceipt, error) {
	return nil, ImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash api.EthHash, txIndex api.EthUint64) (api.EthTx, error) {
	return api.EthTx{}, ImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum api.EthUint64, txIndex api.EthUint64) (api.EthTx, error) {
	return api.EthTx{}, ImplementMe
}

func (e *EthModuleDummy) EthGetCode(ctx context.Context, address api.EthAddress, blkOpt string) (api.EthBytes, error) {
	return nil, ImplementMe
}

func (e *EthModuleDummy) EthGetStorageAt(ctx context.Context, address api.EthAddress, position api.EthBytes, blkParam string) (api.EthBytes, error) {
	return nil, ImplementMe
}

func (e *EthModuleDummy) EthGetBalance(ctx context.Context, address api.EthAddress, blkParam string) (api.EthBigInt, error) {
	return api.EthBigIntZero, ImplementMe
}

func (e *EthModuleDummy) EthFeeHistory(ctx context.Context, blkCount api.EthUint64, newestBlk string, rewardPercentiles []int64) (api.EthFeeHistory, error) {
	return api.EthFeeHistory{}, ImplementMe
}

func (e *EthModuleDummy) EthChainId(ctx context.Context) (api.EthUint64, error) {
	return 0, ImplementMe
}

func (e *EthModuleDummy) NetVersion(ctx context.Context) (string, error) {
	return "", ImplementMe
}

func (e *EthModuleDummy) NetListening(ctx context.Context) (bool, error) {
	return false, ImplementMe
}

func (e *EthModuleDummy) EthProtocolVersion(ctx context.Context) (api.EthUint64, error) {
	return 0, ImplementMe
}

func (e *EthModuleDummy) EthGasPrice(ctx context.Context) (api.EthBigInt, error) {
	return api.EthBigIntZero, ImplementMe
}

func (e *EthModuleDummy) EthEstimateGas(ctx context.Context, tx api.EthCall) (api.EthUint64, error) {
	return 0, ImplementMe
}

func (e *EthModuleDummy) EthCall(ctx context.Context, tx api.EthCall, blkParam string) (api.EthBytes, error) {
	return nil, ImplementMe
}

func (e *EthModuleDummy) EthMaxPriorityFeePerGas(ctx context.Context) (api.EthBigInt, error) {
	return api.EthBigIntZero, ImplementMe
}

func (e *EthModuleDummy) EthSendRawTransaction(ctx context.Context, rawTx api.EthBytes) (api.EthHash, error) {
	return api.EthHash{}, ImplementMe
}
