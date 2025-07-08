package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var ErrTooManyFilters = errors.New("too many subscriptions and filters per connection")

func (pv1 *reverseProxyV1) EthAccounts(context.Context) ([]ethtypes.EthAddress, error) {
	// gateway provides a public API, so it can't hold user accounts
	return []ethtypes.EthAddress{}, nil
}

func (pv1 *reverseProxyV1) EthAddressToFilecoinAddress(ctx context.Context, ethAddress ethtypes.EthAddress) (address.Address, error) {
	return pv1.server.EthAddressToFilecoinAddress(ctx, ethAddress)
}

func (pv1 *reverseProxyV1) FilecoinAddressToEthAddress(ctx context.Context, params jsonrpc.RawParams) (ethtypes.EthAddress, error) {
	// validate params
	_, err := jsonrpc.DecodeParams[ethtypes.FilecoinAddressToEthAddressParams](params)
	if err != nil {
		return ethtypes.EthAddress{}, xerrors.Errorf("decoding params: %w", err)
	}

	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthAddress{}, err
	}

	return pv1.server.FilecoinAddressToEthAddress(ctx, params)
}

func (pv1 *reverseProxyV1) EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return 0, err
	}

	return pv1.server.EthBlockNumber(ctx)
}

func (pv1 *reverseProxyV1) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum string) (ethtypes.EthUint64, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	if err := pv1.checkBlkParam(ctx, blkNum, 0); err != nil {
		return 0, err
	}

	return pv1.server.EthGetBlockTransactionCountByNumber(ctx, blkNum)
}

func (pv1 *reverseProxyV1) tskByEthHash(ctx context.Context, blkHash ethtypes.EthHash) (types.TipSetKey, error) {
	tskCid := blkHash.ToCid()
	tskBlk, err := pv1.ChainReadObj(ctx, tskCid)
	if err != nil {
		return types.EmptyTSK, err
	}
	tsk := new(types.TipSetKey)
	if err := tsk.UnmarshalCBOR(bytes.NewReader(tskBlk)); err != nil {
		return types.EmptyTSK, xerrors.Errorf("cannot unmarshal block into tipset key: %w", err)
	}

	return *tsk, nil
}

func (pv1 *reverseProxyV1) checkBlkHash(ctx context.Context, blkHash ethtypes.EthHash) error {
	tsk, err := pv1.tskByEthHash(ctx, blkHash)
	if err != nil {
		return err
	}

	return pv1.gateway.checkTipSetKey(ctx, tsk)
}

func (pv1 *reverseProxyV1) checkEthBlockParam(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, lookback ethtypes.EthUint64) error {
	// first check if it's a predefined block or a block number
	if blkParam.PredefinedBlock != nil || blkParam.BlockNumber != nil {
		head, err := pv1.ChainHead(ctx)
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

		return pv1.gateway.checkTipSetHeight(head, abi.ChainEpoch(num))
	}

	// otherwise its a block hash
	if blkParam.BlockHash != nil {
		return pv1.checkBlkHash(ctx, *blkParam.BlockHash)
	}

	return xerrors.New("invalid block param")
}

func (pv1 *reverseProxyV1) checkBlkParam(ctx context.Context, blkParam string, lookback ethtypes.EthUint64) error {
	if blkParam == "earliest" {
		// also not supported in node impl
		return xerrors.New("block param \"earliest\" is not supported")
	}

	head, err := pv1.ChainHead(ctx)
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
	return pv1.gateway.checkTipSetHeight(head, abi.ChainEpoch(num))
}

func (pv1 *reverseProxyV1) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return 0, err
	}

	return pv1.server.EthGetBlockTransactionCountByHash(ctx, blkHash)
}

func (pv1 *reverseProxyV1) EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthBlock{}, err
	}

	if err := pv1.checkBlkHash(ctx, blkHash); err != nil {
		return ethtypes.EthBlock{}, err
	}

	return pv1.server.EthGetBlockByHash(ctx, blkHash, fullTxInfo)
}

func (pv1 *reverseProxyV1) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthBlock{}, err
	}

	if err := pv1.checkBlkParam(ctx, blkNum, 0); err != nil {
		return ethtypes.EthBlock{}, err
	}

	return pv1.server.EthGetBlockByNumber(ctx, blkNum, fullTxInfo)
}

func (pv1 *reverseProxyV1) EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv1.checkBlkHash(ctx, blkHash); err != nil {
		return nil, err
	}

	return pv1.server.EthGetTransactionByBlockHashAndIndex(ctx, blkHash, txIndex)
}

func (pv1 *reverseProxyV1) EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum string, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv1.checkBlkParam(ctx, blkNum, 0); err != nil {
		return nil, err
	}

	return pv1.server.EthGetTransactionByBlockNumberAndIndex(ctx, blkNum, txIndex)
}

func (pv1 *reverseProxyV1) EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.EthGetTransactionByHashLimited(ctx, txHash, pv1.gateway.maxMessageLookbackEpochs)
}

func (pv1 *reverseProxyV1) EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	return pv1.server.EthGetTransactionHashByCid(ctx, cid)
}

func (pv1 *reverseProxyV1) EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}

	return pv1.server.EthGetMessageCidByTransactionHash(ctx, txHash)
}

func (pv1 *reverseProxyV1) EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	if err := pv1.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return 0, err
	}

	return pv1.server.EthGetTransactionCount(ctx, sender, blkParam)
}

func (pv1 *reverseProxyV1) EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*ethtypes.EthTxReceipt, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.EthGetTransactionReceiptLimited(ctx, txHash, pv1.gateway.maxMessageLookbackEpochs)
}

func (pv1 *reverseProxyV1) EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv1.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return nil, err
	}

	return pv1.server.EthGetCode(ctx, address, blkParam)
}

func (pv1 *reverseProxyV1) EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv1.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return nil, err
	}

	return pv1.server.EthGetStorageAt(ctx, address, position, blkParam)
}

func (pv1 *reverseProxyV1) EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	if err := pv1.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	return pv1.server.EthGetBalance(ctx, address, blkParam)
}

func (pv1 *reverseProxyV1) EthChainId(ctx context.Context) (ethtypes.EthUint64, error) {
	if err := pv1.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return 0, err
	}

	return pv1.server.EthChainId(ctx)
}

func (pv1 *reverseProxyV1) EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error) {
	if err := pv1.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return ethtypes.EthSyncingResult{}, err
	}

	return pv1.server.EthSyncing(ctx)
}

func (pv1 *reverseProxyV1) NetVersion(ctx context.Context) (string, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return "", err
	}

	return pv1.server.NetVersion(ctx)
}

func (pv1 *reverseProxyV1) NetListening(ctx context.Context) (bool, error) {
	if err := pv1.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return false, err
	}

	return pv1.server.NetListening(ctx)
}

func (pv1 *reverseProxyV1) EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	return pv1.server.EthProtocolVersion(ctx)
}

func (pv1 *reverseProxyV1) EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	return pv1.server.EthGasPrice(ctx)
}

var EthFeeHistoryMaxBlockCount = 128 // this seems to be expensive; todo: figure out what is a good number that works with everything

func (pv1 *reverseProxyV1) EthFeeHistory(ctx context.Context, jparams jsonrpc.RawParams) (ethtypes.EthFeeHistory, error) {
	params, err := jsonrpc.DecodeParams[ethtypes.EthFeeHistoryParams](jparams)
	if err != nil {
		return ethtypes.EthFeeHistory{}, xerrors.Errorf("decoding params: %w", err)
	}

	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFeeHistory{}, err
	}

	if err := pv1.checkBlkParam(ctx, params.NewestBlkNum, params.BlkCount); err != nil {
		return ethtypes.EthFeeHistory{}, err
	}

	if params.BlkCount > ethtypes.EthUint64(EthFeeHistoryMaxBlockCount) {
		return ethtypes.EthFeeHistory{}, xerrors.New("block count too high")
	}

	return pv1.server.EthFeeHistory(ctx, jparams)
}

func (pv1 *reverseProxyV1) EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	return pv1.server.EthMaxPriorityFeePerGas(ctx)
}

func (pv1 *reverseProxyV1) EthEstimateGas(ctx context.Context, jparams jsonrpc.RawParams) (ethtypes.EthUint64, error) {
	// validate params
	_, err := jsonrpc.DecodeParams[ethtypes.EthEstimateGasParams](jparams)
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("decoding params: %w", err)
	}

	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	// todo limit gas? to what?
	return pv1.server.EthEstimateGas(ctx, jparams)
}

func (pv1 *reverseProxyV1) EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv1.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return nil, err
	}

	// todo limit gas? to what?
	return pv1.server.EthCall(ctx, tx, blkParam)
}

func (pv1 *reverseProxyV1) EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthHash{}, err
	}

	// push the message via the untrusted variant which uses MpoolPushUntrusted
	return pv1.server.EthSendRawTransactionUntrusted(ctx, rawTx)
}

func (pv1 *reverseProxyV1) EthGetLogs(ctx context.Context, filter *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if filter.FromBlock != nil {
		if err := pv1.checkBlkParam(ctx, *filter.FromBlock, 0); err != nil {
			return nil, err
		}
	}
	if filter.ToBlock != nil {
		if err := pv1.checkBlkParam(ctx, *filter.ToBlock, 0); err != nil {
			return nil, err
		}
	}
	if filter.BlockHash != nil {
		if err := pv1.checkBlkHash(ctx, *filter.BlockHash); err != nil {
			return nil, err
		}
	}

	return pv1.server.EthGetLogs(ctx, filter)
}

func (pv1 *reverseProxyV1) EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	ft, err := getStatefulTrackerV1(ctx)
	if err != nil {
		return nil, xerrors.Errorf("EthGetFilterChanges not supported: %w", err)
	}

	if !ft.hasFilter(id) {
		return nil, filter.ErrFilterNotFound
	}

	return pv1.server.EthGetFilterChanges(ctx, id)
}

func (pv1 *reverseProxyV1) EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	ft, err := getStatefulTrackerV1(ctx)
	if err != nil {
		return nil, xerrors.Errorf("EthGetFilterLogs not supported: %w", err)
	}

	if !ft.hasFilter(id) {
		return nil, nil
	}

	return pv1.server.EthGetFilterLogs(ctx, id)
}

func (pv1 *reverseProxyV1) EthNewFilter(ctx context.Context, filter *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFilterID{}, err
	}

	return pv1.addUserFilterLimited(ctx, "EthNewFilter", func() (ethtypes.EthFilterID, error) {
		return pv1.server.EthNewFilter(ctx, filter)
	})
}

func (pv1 *reverseProxyV1) EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFilterID{}, err
	}

	return pv1.addUserFilterLimited(ctx, "EthNewBlockFilter", func() (ethtypes.EthFilterID, error) {
		return pv1.server.EthNewBlockFilter(ctx)
	})
}

func (pv1 *reverseProxyV1) EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFilterID{}, err
	}

	return pv1.addUserFilterLimited(ctx, "EthNewPendingTransactionFilter", func() (ethtypes.EthFilterID, error) {
		return pv1.server.EthNewPendingTransactionFilter(ctx)
	})
}

func (pv1 *reverseProxyV1) EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}

	// check if the filter belongs to this connection
	ft, err := getStatefulTrackerV1(ctx)
	if err != nil {
		return false, xerrors.Errorf("EthUninstallFilter not supported: %w", err)
	}

	ft.lk.Lock()
	defer ft.lk.Unlock()

	if _, ok := ft.userFilters[id]; !ok {
		return false, nil
	}

	ok, err := pv1.server.EthUninstallFilter(ctx, id)
	if err != nil {
		// don't delete the filter, it's "stuck" so should still count towards the limit
		log.Warnf("error uninstalling filter: %v", err)
		return false, err
	}

	delete(ft.userFilters, id)
	return ok, nil
}

func (pv1 *reverseProxyV1) EthSubscribe(ctx context.Context, jparams jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error) {
	// validate params
	_, err := jsonrpc.DecodeParams[ethtypes.EthSubscribeParams](jparams)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("decoding params: %w", err)
	}

	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthSubscriptionID{}, err
	}

	if pv1.subscriptions == nil {
		return ethtypes.EthSubscriptionID{}, xerrors.New("EthSubscribe not supported: subscription support not enabled")
	}

	ethCb, ok := jsonrpc.ExtractReverseClient[api.EthSubscriberMethods](ctx)
	if !ok {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("EthSubscribe not supported: connection doesn't support callbacks")
	}

	ft, err := getStatefulTrackerV1(ctx)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("EthSubscribe not supported: %w", err)
	}
	ft.lk.Lock()
	defer ft.lk.Unlock()

	if len(ft.userSubscriptions)+len(ft.userFilters) >= pv1.gateway.ethMaxFiltersPerConn {
		return ethtypes.EthSubscriptionID{}, ErrTooManyFilters
	}

	sub, err := pv1.server.EthSubscribe(ctx, jparams)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, err
	}

	err = pv1.subscriptions.AddSub(ctx, sub, func(ctx context.Context, response *ethtypes.EthSubscriptionResponse) error {
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
		if _, err := pv1.server.EthUnsubscribe(ctx, sub); err != nil {
			log.Warnf("error unsubscribing after connection end: %v", err)
		}
	}

	return sub, err
}

func (pv1 *reverseProxyV1) EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}

	// check if the filter belongs to this connection
	ft, err := getStatefulTrackerV1(ctx)
	if err != nil {
		return false, xerrors.Errorf("EthUnsubscribe not supported: %w", err)
	}
	ft.lk.Lock()
	defer ft.lk.Unlock()

	if _, ok := ft.userSubscriptions[id]; !ok {
		return false, nil
	}

	ok, err := pv1.server.EthUnsubscribe(ctx, id)
	if err != nil {
		// don't delete the subscription, it's "stuck" so should still count towards the limit
		log.Warnf("error unsubscribing: %v", err)
		return false, err
	}

	delete(ft.userSubscriptions, id)

	if pv1.subscriptions != nil {
		pv1.subscriptions.RemoveSub(id)
	}

	return ok, nil
}

func (pv1 *reverseProxyV1) Web3ClientVersion(ctx context.Context) (string, error) {
	if err := pv1.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return "", err
	}

	return pv1.server.Web3ClientVersion(ctx)
}

func (pv1 *reverseProxyV1) EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv1.checkBlkParam(ctx, blkNum, 0); err != nil {
		return nil, err
	}

	return pv1.server.EthTraceBlock(ctx, blkNum)
}

func (pv1 *reverseProxyV1) EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := pv1.checkBlkParam(ctx, blkNum, 0); err != nil {
		return nil, err
	}

	return pv1.server.EthTraceReplayBlockTransactions(ctx, blkNum, traceTypes)
}

func (pv1 *reverseProxyV1) EthTraceTransaction(ctx context.Context, txHash string) ([]*ethtypes.EthTraceTransaction, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	return pv1.server.EthTraceTransaction(ctx, txHash)
}

func (pv1 *reverseProxyV1) EthTraceFilter(ctx context.Context, filter ethtypes.EthTraceFilterCriteria) ([]*ethtypes.EthTraceFilterResult, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if filter.ToBlock != nil {
		if err := pv1.checkBlkParam(ctx, *filter.ToBlock, 0); err != nil {
			return nil, err
		}
	}

	if filter.FromBlock != nil {
		if err := pv1.checkBlkParam(ctx, *filter.FromBlock, 0); err != nil {
			return nil, err
		}
	}

	return pv1.server.EthTraceFilter(ctx, filter)
}

func (pv1 *reverseProxyV1) EthGetBlockReceipts(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) ([]*ethtypes.EthTxReceipt, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.EthGetBlockReceiptsLimited(ctx, blkParam, pv1.gateway.maxMessageLookbackEpochs)
}

func (pv1 *reverseProxyV1) addUserFilterLimited(
	ctx context.Context,
	callName string,
	install func() (ethtypes.EthFilterID, error),
) (ethtypes.EthFilterID, error) {
	ft, err := getStatefulTrackerV1(ctx)
	if err != nil {
		return ethtypes.EthFilterID{}, xerrors.Errorf("%s not supported: %w", callName, err)
	}

	ft.lk.Lock()
	defer ft.lk.Unlock()

	if len(ft.userSubscriptions)+len(ft.userFilters) >= pv1.gateway.ethMaxFiltersPerConn {
		return ethtypes.EthFilterID{}, ErrTooManyFilters
	}

	id, err := install()
	if err != nil {
		return id, err
	}

	ft.userFilters[id] = func() {
		if _, err := pv1.server.EthUninstallFilter(ctx, id); err != nil {
			log.Warnf("error uninstalling filter after connection end: %v", err)
		}
	}

	return id, nil
}

func getStatefulTrackerV1(ctx context.Context) (*statefulCallTracker, error) {
	if jsonrpc.GetConnectionType(ctx) != jsonrpc.ConnectionTypeWS {
		return nil, xerrors.New("stateful methods are only available on websocket connections")
	}

	if ct, ok := ctx.Value(statefulCallTrackerKeyV1).(*statefulCallTracker); !ok {
		return nil, xerrors.New("stateful tracking is not available for this call")
	} else {
		return ct, nil
	}
}

type cleanup func()

type statefulCallTracker struct {
	lk sync.Mutex

	userFilters       map[ethtypes.EthFilterID]cleanup
	userSubscriptions map[ethtypes.EthSubscriptionID]cleanup
}

func (ft *statefulCallTracker) cleanup() {
	ft.lk.Lock()
	defer ft.lk.Unlock()

	for _, cleanup := range ft.userFilters {
		cleanup()
	}
	for _, cleanup := range ft.userSubscriptions {
		cleanup()
	}
}

func (ft *statefulCallTracker) hasFilter(id ethtypes.EthFilterID) bool {
	ft.lk.Lock()
	defer ft.lk.Unlock()

	_, ok := ft.userFilters[id]
	return ok
}

// called per request (ws connection)
func newStatefulCallTracker() *statefulCallTracker {
	return &statefulCallTracker{
		userFilters:       make(map[ethtypes.EthFilterID]cleanup),
		userSubscriptions: make(map[ethtypes.EthSubscriptionID]cleanup),
	}
}
