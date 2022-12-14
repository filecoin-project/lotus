package full

import (
	"context"
	"errors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/eth"
)

var ErrImplementMe = errors.New("Not implemented yet")

type EthModuleDummy struct{}

func (e *EthModuleDummy) EthBlockNumber(ctx context.Context) (eth.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthAccounts(ctx context.Context) ([]eth.EthAddress, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum eth.EthUint64) (eth.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash eth.EthHash) (eth.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockByHash(ctx context.Context, blkHash eth.EthHash, fullTxInfo bool) (eth.EthBlock, error) {
	return eth.EthBlock{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (eth.EthBlock, error) {
	return eth.EthBlock{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByHash(ctx context.Context, txHash *eth.EthHash) (*eth.EthTx, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionCount(ctx context.Context, sender eth.EthAddress, blkOpt string) (eth.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionReceipt(ctx context.Context, txHash eth.EthHash) (*api.EthTxReceipt, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash eth.EthHash, txIndex eth.EthUint64) (eth.EthTx, error) {
	return eth.EthTx{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum eth.EthUint64, txIndex eth.EthUint64) (eth.EthTx, error) {
	return eth.EthTx{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetCode(ctx context.Context, address eth.EthAddress, blkOpt string) (eth.EthBytes, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetStorageAt(ctx context.Context, address eth.EthAddress, position eth.EthBytes, blkParam string) (eth.EthBytes, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBalance(ctx context.Context, address eth.EthAddress, blkParam string) (eth.EthBigInt, error) {
	return eth.EthBigIntZero, ErrImplementMe
}

func (e *EthModuleDummy) EthFeeHistory(ctx context.Context, blkCount eth.EthUint64, newestBlk string, rewardPercentiles []float64) (eth.EthFeeHistory, error) {
	return eth.EthFeeHistory{}, ErrImplementMe
}

func (e *EthModuleDummy) EthChainId(ctx context.Context) (eth.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) NetVersion(ctx context.Context) (string, error) {
	return "", ErrImplementMe
}

func (e *EthModuleDummy) NetListening(ctx context.Context) (bool, error) {
	return false, ErrImplementMe
}

func (e *EthModuleDummy) EthProtocolVersion(ctx context.Context) (eth.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGasPrice(ctx context.Context) (eth.EthBigInt, error) {
	return eth.EthBigIntZero, ErrImplementMe
}

func (e *EthModuleDummy) EthEstimateGas(ctx context.Context, tx eth.EthCall) (eth.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthCall(ctx context.Context, tx eth.EthCall, blkParam string) (eth.EthBytes, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthMaxPriorityFeePerGas(ctx context.Context) (eth.EthBigInt, error) {
	return eth.EthBigIntZero, ErrImplementMe
}

func (e *EthModuleDummy) EthSendRawTransaction(ctx context.Context, rawTx eth.EthBytes) (eth.EthHash, error) {
	return eth.EthHash{}, ErrImplementMe
}
