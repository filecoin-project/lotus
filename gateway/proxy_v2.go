package gateway

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var _ v2api.Gateway = (*reverseProxyV2)(nil)

type reverseProxyV2 struct {
	gateway       *Node
	server        v2api.FullNode
	subscriptions *EthSubHandler
}

func (pv2 *reverseProxyV2) ChainGetTipSet(ctx context.Context, selector types.TipSetSelector) (*types.TipSet, error) {
	if err := pv2.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv2.server.ChainGetTipSet(ctx, selector)
}

func (pv2 *reverseProxyV2) StateGetActor(ctx context.Context, address address.Address, selector types.TipSetSelector) (*types.Actor, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv2.server.StateGetActor(ctx, address, selector)
}

func (pv2 *reverseProxyV2) StateGetID(ctx context.Context, address address.Address, selector types.TipSetSelector) (*address.Address, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv2.server.StateGetID(ctx, address, selector)
}

func (pv2 *reverseProxyV2) EthAddressToFilecoinAddress(ctx context.Context, ethAddress ethtypes.EthAddress) (address.Address, error) {
	return pv2.server.EthAddressToFilecoinAddress(ctx, ethAddress)
}

func (pv2 *reverseProxyV2) FilecoinAddressToEthAddress(ctx context.Context, params jsonrpc.RawParams) (ethtypes.EthAddress, error) {
	_, err := jsonrpc.DecodeParams[ethtypes.FilecoinAddressToEthAddressParams](params)
	if err != nil {
		return ethtypes.EthAddress{}, xerrors.Errorf("decoding params: %w", err)
	}

	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthAddress{}, err
	}
	return pv2.server.FilecoinAddressToEthAddress(ctx, params)
}

func (pv2 *reverseProxyV2) Web3ClientVersion(ctx context.Context) (string, error) {
	if err := pv2.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return "", err
	}

	return pv2.server.Web3ClientVersion(ctx)
}

func (pv2 *reverseProxyV2) EthChainId(ctx context.Context) (ethtypes.EthUint64, error) {
	if err := pv2.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return 0, err
	}

	return pv2.server.EthChainId(ctx)
}

func (pv2 *reverseProxyV2) NetVersion(ctx context.Context) (string, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return "", err
	}

	return pv2.server.NetVersion(ctx)
}

func (pv2 *reverseProxyV2) NetListening(ctx context.Context) (bool, error) {
	if err := pv2.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return false, err
	}

	return pv2.server.NetListening(ctx)
}

func (pv2 *reverseProxyV2) EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	return pv2.server.EthProtocolVersion(ctx)
}

func (pv2 *reverseProxyV2) EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error) {
	if err := pv2.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return ethtypes.EthSyncingResult{}, err
	}

	return pv2.server.EthSyncing(ctx)
}

func (pv2 *reverseProxyV2) EthAccounts(context.Context) ([]ethtypes.EthAddress, error) {
	// gateway provides public API, so it can't hold user accounts
	return []ethtypes.EthAddress{}, nil
}

func (pv2 *reverseProxyV2) EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthHash{}, err
	}

	// push the message via the untrusted variant which uses MpoolPushUntrusted
	return pv2.server.EthSendRawTransactionUntrusted(ctx, rawTx)
}

func (pv2 *reverseProxyV2) EthSendRawTransactionUntrusted(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthHash{}, err
	}
	return pv2.server.EthSendRawTransactionUntrusted(ctx, rawTx)
}

func (pv2 *reverseProxyV2) EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error) {
	if err := pv2.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return 0, err
	}

	return pv2.server.EthBlockNumber(ctx)
}

func (pv2 *reverseProxyV2) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum string) (ethtypes.EthUint64, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	if err := pv2.checkBlkParam(ctx, blkNum, 0); err != nil {
		return 0, err
	}

	return pv2.server.EthGetBlockTransactionCountByNumber(ctx, blkNum)
}

func (pv2 *reverseProxyV2) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error) {
	if err := pv2.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return 0, err
	}

	return pv2.server.EthGetBlockTransactionCountByHash(ctx, blkHash)
}

func (pv2 *reverseProxyV2) EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthBlock{}, err
	}

	if err := pv2.checkBlkHash(ctx, blkHash); err != nil {
		return ethtypes.EthBlock{}, err
	}

	return pv2.server.EthGetBlockByHash(ctx, blkHash, fullTxInfo)
}

func (pv2 *reverseProxyV2) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthBlock{}, err
	}

	if err := pv2.checkBlkParam(ctx, blkNum, 0); err != nil {
		return ethtypes.EthBlock{}, err
	}

	return pv2.server.EthGetBlockByNumber(ctx, blkNum, fullTxInfo)
}

func (pv2 *reverseProxyV2) EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv2.server.EthGetTransactionByHashLimited(ctx, txHash, pv2.gateway.maxMessageLookbackEpochs)
}

func (pv2 *reverseProxyV2) EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv2.server.EthGetTransactionByHashLimited(ctx, txHash, limit)
}

func (pv2 *reverseProxyV2) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error) {
	if err := pv2.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv2.checkBlkHash(ctx, blkHash); err != nil {
		return nil, err
	}

	return pv2.server.EthGetTransactionByBlockHashAndIndex(ctx, blkHash, txIndex)
}

func (pv2 *reverseProxyV2) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum string, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv2.checkBlkParam(ctx, blkNum, 0); err != nil {
		return nil, err
	}

	return pv2.server.EthGetTransactionByBlockNumberAndIndex(ctx, blkNum, txIndex)
}

func (pv2 *reverseProxyV2) EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error) {
	if err := pv2.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}

	return pv2.server.EthGetMessageCidByTransactionHash(ctx, txHash)
}

func (pv2 *reverseProxyV2) EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	return pv2.server.EthGetTransactionHashByCid(ctx, cid)
}

func (pv2 *reverseProxyV2) EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	if err := pv2.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return 0, err
	}

	return pv2.server.EthGetTransactionCount(ctx, sender, blkParam)
}

func (pv2 *reverseProxyV2) EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*ethtypes.EthTxReceipt, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv2.server.EthGetTransactionReceiptLimited(ctx, txHash, pv2.gateway.maxMessageLookbackEpochs)
}

func (pv2 *reverseProxyV2) EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTxReceipt, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv2.server.EthGetTransactionReceiptLimited(ctx, txHash, limit)
}

func (pv2 *reverseProxyV2) EthGetBlockReceipts(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) ([]*ethtypes.EthTxReceipt, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv2.server.EthGetBlockReceiptsLimited(ctx, blkParam, pv2.gateway.maxMessageLookbackEpochs)
}

func (pv2 *reverseProxyV2) EthGetBlockReceiptsLimited(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, limit abi.ChainEpoch) ([]*ethtypes.EthTxReceipt, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv2.server.EthGetBlockReceiptsLimited(ctx, blkParam, limit)
}

func (pv2 *reverseProxyV2) EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv2.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return nil, err
	}

	return pv2.server.EthGetCode(ctx, address, blkParam)
}

func (pv2 *reverseProxyV2) EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv2.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return nil, err
	}

	return pv2.server.EthGetStorageAt(ctx, address, position, blkParam)
}

func (pv2 *reverseProxyV2) EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	if err := pv2.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	return pv2.server.EthGetBalance(ctx, address, blkParam)
}

func (pv2 *reverseProxyV2) EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv2.checkBlkParam(ctx, blkNum, 0); err != nil {
		return nil, err
	}

	return pv2.server.EthTraceBlock(ctx, blkNum)
}

func (pv2 *reverseProxyV2) EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv2.checkBlkParam(ctx, blkNum, 0); err != nil {
		return nil, err
	}

	return pv2.server.EthTraceReplayBlockTransactions(ctx, blkNum, traceTypes)
}

func (pv2 *reverseProxyV2) EthTraceTransaction(ctx context.Context, txHash string) ([]*ethtypes.EthTraceTransaction, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	return pv2.server.EthTraceTransaction(ctx, txHash)
}

func (pv2 *reverseProxyV2) EthTraceFilter(ctx context.Context, filter ethtypes.EthTraceFilterCriteria) ([]*ethtypes.EthTraceFilterResult, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if filter.ToBlock != nil {
		if err := pv2.checkBlkParam(ctx, *filter.ToBlock, 0); err != nil {
			return nil, err
		}
	}

	if filter.FromBlock != nil {
		if err := pv2.checkBlkParam(ctx, *filter.FromBlock, 0); err != nil {
			return nil, err
		}
	}

	return pv2.server.EthTraceFilter(ctx, filter)
}

func (pv2 *reverseProxyV2) EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error) {
	if err := pv2.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	return pv2.server.EthGasPrice(ctx)
}

func (pv2 *reverseProxyV2) EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error) {
	params, err := jsonrpc.DecodeParams[ethtypes.EthFeeHistoryParams](p)
	if err != nil {
		return ethtypes.EthFeeHistory{}, xerrors.Errorf("decoding params: %w", err)
	}

	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFeeHistory{}, err
	}

	if err := pv2.checkBlkParam(ctx, params.NewestBlkNum, params.BlkCount); err != nil {
		return ethtypes.EthFeeHistory{}, err
	}

	if params.BlkCount > ethtypes.EthUint64(EthFeeHistoryMaxBlockCount) {
		return ethtypes.EthFeeHistory{}, xerrors.New("block count too high")
	}

	return pv2.server.EthFeeHistory(ctx, p)
}

func (pv2 *reverseProxyV2) EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error) {
	if err := pv2.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	return pv2.server.EthMaxPriorityFeePerGas(ctx)
}

func (pv2 *reverseProxyV2) EthEstimateGas(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthUint64, error) {
	// validate params
	_, err := jsonrpc.DecodeParams[ethtypes.EthEstimateGasParams](p)
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("decoding params: %w", err)
	}

	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	// todo limit gas? to what?
	return pv2.server.EthEstimateGas(ctx, p)
}

func (pv2 *reverseProxyV2) EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv2.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return nil, err
	}

	// todo limit gas? to what?
	return pv2.server.EthCall(ctx, tx, blkParam)
}

func (pv2 *reverseProxyV2) EthGetLogs(ctx context.Context, filter *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if filter.FromBlock != nil {
		if err := pv2.checkBlkParam(ctx, *filter.FromBlock, 0); err != nil {
			return nil, err
		}
	}
	if filter.ToBlock != nil {
		if err := pv2.checkBlkParam(ctx, *filter.ToBlock, 0); err != nil {
			return nil, err
		}
	}
	if filter.BlockHash != nil {
		if err := pv2.checkBlkHash(ctx, *filter.BlockHash); err != nil {
			return nil, err
		}
	}

	return pv2.server.EthGetLogs(ctx, filter)
}

func (pv2 *reverseProxyV2) EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFilterID{}, err
	}

	return pv2.addUserFilterLimited(ctx, "EthNewBlockFilter", func() (ethtypes.EthFilterID, error) {
		return pv2.server.EthNewBlockFilter(ctx)
	})
}

func (pv2 *reverseProxyV2) EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFilterID{}, err
	}

	return pv2.addUserFilterLimited(ctx, "EthNewPendingTransactionFilter", func() (ethtypes.EthFilterID, error) {
		return pv2.server.EthNewPendingTransactionFilter(ctx)
	})
}

func (pv2 *reverseProxyV2) EthNewFilter(ctx context.Context, filter *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFilterID{}, err
	}

	return pv2.addUserFilterLimited(ctx, "EthNewFilter", func() (ethtypes.EthFilterID, error) {
		return pv2.server.EthNewFilter(ctx, filter)
	})
}

func (pv2 *reverseProxyV2) EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}

	// check if the filter belongs to this connection
	ft, err := getStatefulTrackerV2(ctx)
	if err != nil {
		return false, xerrors.Errorf("EthUninstallFilter not supported: %w", err)
	}

	ft.lk.Lock()
	defer ft.lk.Unlock()

	if _, ok := ft.userFilters[id]; !ok {
		return false, nil
	}

	ok, err := pv2.server.EthUninstallFilter(ctx, id)
	if err != nil {
		// don't delete the filter, it's "stuck" so should still count towards the limit
		log.Warnf("error uninstalling filter: %v", err)
		return false, err
	}

	delete(ft.userFilters, id)
	return ok, nil
}

func (pv2 *reverseProxyV2) EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	ft, err := getStatefulTrackerV2(ctx)
	if err != nil {
		return nil, xerrors.Errorf("EthGetFilterChanges not supported: %w", err)
	}

	if !ft.hasFilter(id) {
		return nil, filter.ErrFilterNotFound
	}

	return pv2.server.EthGetFilterChanges(ctx, id)
}

func (pv2 *reverseProxyV2) EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	ft, err := getStatefulTrackerV2(ctx)
	if err != nil {
		return nil, xerrors.Errorf("EthGetFilterLogs not supported: %w", err)
	}

	if !ft.hasFilter(id) {
		return nil, nil
	}

	return pv2.server.EthGetFilterLogs(ctx, id)
}

func (pv2 *reverseProxyV2) EthSubscribe(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error) {
	// validate params
	_, err := jsonrpc.DecodeParams[ethtypes.EthSubscribeParams](p)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("decoding params: %w", err)
	}

	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthSubscriptionID{}, err
	}

	if pv2.subscriptions == nil {
		return ethtypes.EthSubscriptionID{}, xerrors.New("EthSubscribe not supported: subscription support not enabled")
	}

	ethCb, ok := jsonrpc.ExtractReverseClient[api.EthSubscriberMethods](ctx)
	if !ok {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("EthSubscribe not supported: connection doesn't support callbacks")
	}

	ft, err := getStatefulTrackerV2(ctx)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("EthSubscribe not supported: %w", err)
	}
	ft.lk.Lock()
	defer ft.lk.Unlock()

	if len(ft.userSubscriptions)+len(ft.userFilters) >= pv2.gateway.ethMaxFiltersPerConn {
		return ethtypes.EthSubscriptionID{}, ErrTooManyFilters
	}

	sub, err := pv2.server.EthSubscribe(ctx, p)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, err
	}

	err = pv2.subscriptions.AddSub(ctx, sub, func(ctx context.Context, response *ethtypes.EthSubscriptionResponse) error {
		outParam, err := json.Marshal(response)
		if err != nil {
			return err
		}

		return ethCb.EthSubscription(ctx, outParam)
	})
	if err != nil {
		return ethtypes.EthSubscriptionID{}, err
	}

	ft.userSubscriptions[sub] = func() {
		if _, err := pv2.server.EthUnsubscribe(ctx, sub); err != nil {
			log.Warnf("error unsubscribing after connection end: %v", err)
		}
	}

	return sub, err
}

func (pv2 *reverseProxyV2) EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error) {
	if err := pv2.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}

	// check if the filter belongs to this connection
	ft, err := getStatefulTrackerV2(ctx)
	if err != nil {
		return false, xerrors.Errorf("EthUnsubscribe not supported: %w", err)
	}
	ft.lk.Lock()
	defer ft.lk.Unlock()

	if _, ok := ft.userSubscriptions[id]; !ok {
		return false, nil
	}

	ok, err := pv2.server.EthUnsubscribe(ctx, id)
	if err != nil {
		// don't delete the subscription, it's "stuck" so should still count towards the limit
		log.Warnf("error unsubscribing: %v", err)
		return false, err
	}

	delete(ft.userSubscriptions, id)

	if pv2.subscriptions != nil {
		pv2.subscriptions.RemoveSub(id)
	}

	return ok, nil
}

func (pv2 *reverseProxyV2) Discover(context.Context) (apitypes.OpenRPCDocument, error) {
	return build.OpenRPCDiscoverJSON_GatewayV2(), nil
}

func getStatefulTrackerV2(ctx context.Context) (*statefulCallTracker, error) {
	if jsonrpc.GetConnectionType(ctx) != jsonrpc.ConnectionTypeWS {
		return nil, xerrors.New("stateful methods are only available on websocket connections")
	}

	if ct, ok := ctx.Value(statefulCallTrackerKeyV2).(*statefulCallTracker); !ok {
		return nil, xerrors.New("stateful tracking is not available for this call")
	} else {
		return ct, nil
	}
}

func (pv2 *reverseProxyV2) addUserFilterLimited(
	ctx context.Context,
	callName string,
	install func() (ethtypes.EthFilterID, error),
) (ethtypes.EthFilterID, error) {
	ft, err := getStatefulTrackerV2(ctx)
	if err != nil {
		return ethtypes.EthFilterID{}, xerrors.Errorf("%s not supported: %w", callName, err)
	}

	ft.lk.Lock()
	defer ft.lk.Unlock()

	if len(ft.userSubscriptions)+len(ft.userFilters) >= pv2.gateway.ethMaxFiltersPerConn {
		return ethtypes.EthFilterID{}, ErrTooManyFilters
	}

	id, err := install()
	if err != nil {
		return id, err
	}

	ft.userFilters[id] = func() {
		if _, err := pv2.EthUninstallFilter(ctx, id); err != nil {
			log.Warnf("error uninstalling filter after connection end: %v", err)
		}
	}

	return id, nil
}

func (pv2 *reverseProxyV2) tskByEthHash(ctx context.Context, blkHash ethtypes.EthHash) (types.TipSetKey, error) {
	// Fall back to V2 to convert the EthHash to a TipSetKey. Because, v2 has not
	// implemented Filecoin.ChainReadObj yet.

	// TODO: Once v2 APIs offer Filecoin.ChainReadObj, replace this with direct call
	//       to whatever the equivalent v2 would be.
	return pv2.gateway.v1Proxy.tskByEthHash(ctx, blkHash)
}

func (pv2 *reverseProxyV2) checkBlkHash(ctx context.Context, blkHash ethtypes.EthHash) error {
	tsk, err := pv2.tskByEthHash(ctx, blkHash)
	if err != nil {
		return err
	}
	return pv2.gateway.checkTipSetKey(ctx, tsk)
}

func (pv2 *reverseProxyV2) checkEthBlockParam(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, lookback ethtypes.EthUint64) error {
	// first check if it's a predefined block or a block number
	if blkParam.PredefinedBlock != nil || blkParam.BlockNumber != nil {
		head, err := pv2.ChainGetTipSet(ctx, types.TipSetSelectors.Latest)
		if err != nil {
			return err
		}

		var num ethtypes.EthUint64
		if blkParam.PredefinedBlock != nil {
			if *blkParam.PredefinedBlock == "earliest" {
				return xerrors.New("block param \"earliest\" is not supported")
			} else if *blkParam.PredefinedBlock == "pending" || *blkParam.PredefinedBlock == "latest" {
				// Head is always ok.
				if lookback == 0 {
					return nil
				}

				if lookback <= ethtypes.EthUint64(head.Height()) {
					num = ethtypes.EthUint64(head.Height()) - lookback
				}
			}
		} else {
			num = *blkParam.BlockNumber
		}

		return pv2.gateway.checkTipSetHeight(head, abi.ChainEpoch(num))
	}

	// otherwise its a block hash
	if blkParam.BlockHash != nil {
		return pv2.checkBlkHash(ctx, *blkParam.BlockHash)
	}

	return xerrors.New("invalid block param")
}

func (pv2 *reverseProxyV2) checkBlkParam(ctx context.Context, blkParam string, lookback ethtypes.EthUint64) error {
	if blkParam == "earliest" {
		// also not supported in node impl
		return xerrors.New(`block param "earliest" is not supported`)
	}

	head, err := pv2.ChainGetTipSet(ctx, types.TipSetSelectors.Latest)
	if err != nil {
		return err
	}

	var num ethtypes.EthUint64
	switch blkParam {
	case "pending", "latest":
		// Head is always ok.
		if lookback == 0 {
			return nil
		}
		// Can't look beyond 0 anyways.
		if lookback > ethtypes.EthUint64(head.Height()) {
			break
		}
		num = ethtypes.EthUint64(head.Height()) - lookback

	// "safe" and "finalized" resolved `num` will not accurate for v2 APIs with F3 active; we would
	// need to query the F3 APIs to get the correct value, but for now we'll go with worst-case and
	// if the lookback limit is very short (2880 by default) then these will fail
	case "safe":
		num = ethtypes.EthUint64(head.Height()) - lookback - ethtypes.EthUint64(ethtypes.SafeEpochDelay)
	case "finalized":
		num = ethtypes.EthUint64(head.Height()) - lookback - ethtypes.EthUint64(policy.ChainFinality)
	default:
		if err := num.UnmarshalJSON([]byte(`"` + blkParam + `"`)); err != nil {
			return fmt.Errorf("cannot parse block number: %v", err)
		}

	}
	return pv2.gateway.checkTipSetHeight(head, abi.ChainEpoch(num))
}
