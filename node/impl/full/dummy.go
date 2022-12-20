package full

import (
	"context"
	"errors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var ErrImplementMe = errors.New("Not implemented yet")

type EthModuleDummy struct{}

func (e *EthModuleDummy) EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthAccounts(ctx context.Context) ([]ethtypes.EthAddress, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum ethtypes.EthUint64) (ethtypes.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error) {
	return ethtypes.EthBlock{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error) {
	return ethtypes.EthBlock{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkOpt string) (ethtypes.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (ethtypes.EthTx, error) {
	return ethtypes.EthTx{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum ethtypes.EthUint64, txIndex ethtypes.EthUint64) (ethtypes.EthTx, error) {
	return ethtypes.EthTx{}, ErrImplementMe
}

func (e *EthModuleDummy) EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkOpt string) (ethtypes.EthBytes, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam string) (ethtypes.EthBytes, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam string) (ethtypes.EthBigInt, error) {
	return ethtypes.EthBigIntZero, ErrImplementMe
}

func (e *EthModuleDummy) EthFeeHistory(ctx context.Context, blkCount ethtypes.EthUint64, newestBlk string, rewardPercentiles []float64) (ethtypes.EthFeeHistory, error) {
	return ethtypes.EthFeeHistory{}, ErrImplementMe
}

func (e *EthModuleDummy) EthChainId(ctx context.Context) (ethtypes.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) NetVersion(ctx context.Context) (string, error) {
	return "", ErrImplementMe
}

func (e *EthModuleDummy) NetListening(ctx context.Context) (bool, error) {
	return false, ErrImplementMe
}

func (e *EthModuleDummy) EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error) {
	return ethtypes.EthBigIntZero, ErrImplementMe
}

func (e *EthModuleDummy) EthEstimateGas(ctx context.Context, tx ethtypes.EthCall) (ethtypes.EthUint64, error) {
	return 0, ErrImplementMe
}

func (e *EthModuleDummy) EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam string) (ethtypes.EthBytes, error) {
	return nil, ErrImplementMe
}

func (e *EthModuleDummy) EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error) {
	return ethtypes.EthBigIntZero, ErrImplementMe
}

func (e *EthModuleDummy) EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	return ethtypes.EthHash{}, ErrImplementMe
}
