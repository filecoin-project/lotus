package full

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var ErrModuleDisabled = errors.New("module disabled, enable with Fevm.EnableEthRPC / LOTUS_FEVM_ENABLEETHPRC")

type EthModuleDummy struct{}

func (e *EthModuleDummy) EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error) {
	return nil, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error) {
	return nil, ErrModuleDisabled
}

func (e *EthModuleDummy) EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}

func (e *EthModuleDummy) EthAccounts(ctx context.Context) ([]ethtypes.EthAddress, error) {
	return nil, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum ethtypes.EthUint64) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error) {
	return ethtypes.EthBlock{}, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error) {
	return ethtypes.EthBlock{}, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error) {
	return nil, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkOpt string) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error) {
	return nil, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (ethtypes.EthTx, error) {
	return ethtypes.EthTx{}, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum ethtypes.EthUint64, txIndex ethtypes.EthUint64) (ethtypes.EthTx, error) {
	return ethtypes.EthTx{}, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkOpt string) (ethtypes.EthBytes, error) {
	return nil, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam string) (ethtypes.EthBytes, error) {
	return nil, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam string) (ethtypes.EthBigInt, error) {
	return ethtypes.EthBigIntZero, ErrModuleDisabled
}

func (e *EthModuleDummy) EthFeeHistory(ctx context.Context, blkCount ethtypes.EthUint64, newestBlk string, rewardPercentiles []float64) (ethtypes.EthFeeHistory, error) {
	return ethtypes.EthFeeHistory{}, ErrModuleDisabled
}

func (e *EthModuleDummy) EthChainId(ctx context.Context) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}

func (e *EthModuleDummy) NetVersion(ctx context.Context) (string, error) {
	return "", ErrModuleDisabled
}

func (e *EthModuleDummy) NetListening(ctx context.Context) (bool, error) {
	return false, ErrModuleDisabled
}

func (e *EthModuleDummy) EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}

func (e *EthModuleDummy) EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error) {
	return ethtypes.EthBigIntZero, ErrModuleDisabled
}

func (e *EthModuleDummy) EthEstimateGas(ctx context.Context, tx ethtypes.EthCall) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}

func (e *EthModuleDummy) EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam string) (ethtypes.EthBytes, error) {
	return nil, ErrModuleDisabled
}

func (e *EthModuleDummy) EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error) {
	return ethtypes.EthBigIntZero, ErrModuleDisabled
}

func (e *EthModuleDummy) EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	return ethtypes.EthHash{}, ErrModuleDisabled
}

var _ EthModuleAPI = &EthModuleDummy{}
