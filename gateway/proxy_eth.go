package gateway

import (
	"context"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
)

func (gw *Node) EthAccounts(ctx context.Context) ([]api.EthAddress, error) {
	// gateway provides public API, so it can't hold user accounts
	return []api.EthAddress{}, nil
}

func (gw *Node) EthBlockNumber(ctx context.Context) (api.EthUint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum api.EthUint64) (api.EthUint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash api.EthHash) (api.EthUint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetBlockByHash(ctx context.Context, blkHash api.EthHash, fullTxInfo bool) (api.EthBlock, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthBlock{}, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (api.EthBlock, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthBlock{}, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetTransactionByHash(ctx context.Context, txHash *api.EthHash) (*api.EthTx, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetTransactionCount(ctx context.Context, sender api.EthAddress, blkOpt string) (api.EthUint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}
	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetTransactionReceipt(ctx context.Context, txHash api.EthHash) (*api.EthTxReceipt, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash api.EthHash, txIndex api.EthUint64) (api.EthTx, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthTx{}, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum api.EthUint64, txIndex api.EthUint64) (api.EthTx, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthTx{}, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetCode(ctx context.Context, address api.EthAddress, blkOpt string) (api.EthBytes, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetStorageAt(ctx context.Context, address api.EthAddress, position api.EthBytes, blkParam string) (api.EthBytes, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetBalance(ctx context.Context, address api.EthAddress, blkParam string) (api.EthBigInt, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthBigInt(big.Zero()), err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthChainId(ctx context.Context) (api.EthUint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}
	//TODO implement me
	panic("implement me")
}

func (gw *Node) NetVersion(ctx context.Context) (string, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return "", err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) NetListening(ctx context.Context) (bool, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthProtocolVersion(ctx context.Context) (api.EthUint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}
	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGasPrice(ctx context.Context) (api.EthBigInt, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthBigInt(big.Zero()), err
	}
	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthFeeHistory(ctx context.Context, blkCount api.EthUint64, newestBlk string, rewardPercentiles []float64) (api.EthFeeHistory, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthFeeHistory{}, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthMaxPriorityFeePerGas(ctx context.Context) (api.EthBigInt, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthBigInt(big.Zero()), err
	}
	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthEstimateGas(ctx context.Context, tx api.EthCall) (api.EthUint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}
	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthCall(ctx context.Context, tx api.EthCall, blkParam string) (api.EthBytes, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthSendRawTransaction(ctx context.Context, rawTx api.EthBytes) (api.EthHash, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthHash{}, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetLogs(ctx context.Context, filter *api.EthFilterSpec) (*api.EthFilterResult, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetFilterChanges(ctx context.Context, id api.EthFilterID) (*api.EthFilterResult, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthGetFilterLogs(ctx context.Context, id api.EthFilterID) (*api.EthFilterResult, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	//TODO implement me
	panic("implement me")
}

/* FILTERS: Those are stateful.. figure out how to properly either bind them to users, or time out? */

func (gw *Node) EthNewFilter(ctx context.Context, filter *api.EthFilterSpec) (api.EthFilterID, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthFilterID{}, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthNewBlockFilter(ctx context.Context) (api.EthFilterID, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthFilterID{}, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthNewPendingTransactionFilter(ctx context.Context) (api.EthFilterID, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.EthFilterID{}, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthUninstallFilter(ctx context.Context, id api.EthFilterID) (bool, error) {
	// todo rate limit this?
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthSubscribe(ctx context.Context, eventType string, params *api.EthSubscriptionParams) (<-chan api.EthSubscriptionResponse, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	//TODO implement me
	panic("implement me")
}

func (gw *Node) EthUnsubscribe(ctx context.Context, id api.EthSubscriptionID) (bool, error) {
	// todo should we actually rate limit this
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}

	//TODO implement me
	panic("implement me")
}
