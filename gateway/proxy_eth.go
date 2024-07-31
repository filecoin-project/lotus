package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

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

func (gw *Node) EthAccounts(ctx context.Context) ([]ethtypes.EthAddress, error) {
	// gateway provides public API, so it can't hold user accounts
	return []ethtypes.EthAddress{}, nil
}

func (gw *Node) EthAddressToFilecoinAddress(ctx context.Context, ethAddress ethtypes.EthAddress) (address.Address, error) {
	return gw.target.EthAddressToFilecoinAddress(ctx, ethAddress)
}

func (gw *Node) FilecoinAddressToEthAddress(ctx context.Context, params jsonrpc.RawParams) (ethtypes.EthAddress, error) {
	// validate params
	_, err := jsonrpc.DecodeParams[ethtypes.FilecoinAddressToEthAddressParams](params)
	if err != nil {
		return ethtypes.EthAddress{}, xerrors.Errorf("decoding params: %w", err)
	}

	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthAddress{}, err
	}

	return gw.target.FilecoinAddressToEthAddress(ctx, params)
}

func (gw *Node) EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return 0, err
	}

	return gw.target.EthBlockNumber(ctx)
}

func (gw *Node) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum ethtypes.EthUint64) (ethtypes.EthUint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	head, err := gw.target.ChainHead(ctx)
	if err != nil {
		return 0, err
	}
	if err := gw.checkTipsetHeight(head, abi.ChainEpoch(blkNum)); err != nil {
		return 0, err
	}

	return gw.target.EthGetBlockTransactionCountByNumber(ctx, blkNum)
}

func (gw *Node) tskByEthHash(ctx context.Context, blkHash ethtypes.EthHash) (types.TipSetKey, error) {
	tskCid := blkHash.ToCid()
	tskBlk, err := gw.target.ChainReadObj(ctx, tskCid)
	if err != nil {
		return types.EmptyTSK, err
	}
	tsk := new(types.TipSetKey)
	if err := tsk.UnmarshalCBOR(bytes.NewReader(tskBlk)); err != nil {
		return types.EmptyTSK, xerrors.Errorf("cannot unmarshal block into tipset key: %w", err)
	}

	return *tsk, nil
}

func (gw *Node) checkBlkHash(ctx context.Context, blkHash ethtypes.EthHash) error {
	tsk, err := gw.tskByEthHash(ctx, blkHash)
	if err != nil {
		return err
	}

	return gw.checkTipsetKey(ctx, tsk)
}

func (gw *Node) checkEthBlockParam(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, lookback ethtypes.EthUint64) error {
	// first check if its a predefined block or a block number
	if blkParam.PredefinedBlock != nil || blkParam.BlockNumber != nil {
		head, err := gw.target.ChainHead(ctx)
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

		return gw.checkTipsetHeight(head, abi.ChainEpoch(num))
	}

	// otherwise its a block hash
	if blkParam.BlockHash != nil {
		return gw.checkBlkHash(ctx, *blkParam.BlockHash)
	}

	return xerrors.New("invalid block param")
}

func (gw *Node) checkBlkParam(ctx context.Context, blkParam string, lookback ethtypes.EthUint64) error {
	if blkParam == "earliest" {
		// also not supported in node impl
		return xerrors.New("block param \"earliest\" is not supported")
	}

	head, err := gw.target.ChainHead(ctx)
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
	case "safe":
		num = ethtypes.EthUint64(head.Height()) - lookback - ethtypes.EthUint64(ethtypes.SafeEpochDelay)
	case "finalized":
		num = ethtypes.EthUint64(head.Height()) - lookback - ethtypes.EthUint64(policy.ChainFinality)
	default:
		if err := num.UnmarshalJSON([]byte(`"` + blkParam + `"`)); err != nil {
			return fmt.Errorf("cannot parse block number: %v", err)
		}

	}
	return gw.checkTipsetHeight(head, abi.ChainEpoch(num))
}

func (gw *Node) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return 0, err
	}

	return gw.target.EthGetBlockTransactionCountByHash(ctx, blkHash)
}

func (gw *Node) EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthBlock{}, err
	}

	if err := gw.checkBlkHash(ctx, blkHash); err != nil {
		return ethtypes.EthBlock{}, err
	}

	return gw.target.EthGetBlockByHash(ctx, blkHash, fullTxInfo)
}

func (gw *Node) EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthBlock{}, err
	}

	if err := gw.checkBlkParam(ctx, blkNum, 0); err != nil {
		return ethtypes.EthBlock{}, err
	}

	return gw.target.EthGetBlockByNumber(ctx, blkNum, fullTxInfo)
}

func (gw *Node) EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error) {
	return gw.target.EthGetTransactionByHashLimited(ctx, txHash, api.LookbackNoLimit)
}

func (gw *Node) EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if limit == api.LookbackNoLimit {
		limit = gw.stateWaitLookbackLimit
	}
	if gw.stateWaitLookbackLimit != api.LookbackNoLimit && limit > gw.stateWaitLookbackLimit {
		limit = gw.stateWaitLookbackLimit
	}

	return gw.target.EthGetTransactionByHashLimited(ctx, txHash, limit)
}

func (gw *Node) EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	return gw.target.EthGetTransactionHashByCid(ctx, cid)
}

func (gw *Node) EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}

	return gw.target.EthGetMessageCidByTransactionHash(ctx, txHash)
}

func (gw *Node) EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	if err := gw.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return 0, err
	}

	return gw.target.EthGetTransactionCount(ctx, sender, blkParam)
}

func (gw *Node) EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*api.EthTxReceipt, error) {
	return gw.EthGetTransactionReceiptLimited(ctx, txHash, api.LookbackNoLimit)
}

func (gw *Node) EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*api.EthTxReceipt, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if limit == api.LookbackNoLimit {
		limit = gw.stateWaitLookbackLimit
	}
	if gw.stateWaitLookbackLimit != api.LookbackNoLimit && limit > gw.stateWaitLookbackLimit {
		limit = gw.stateWaitLookbackLimit
	}

	return gw.target.EthGetTransactionReceiptLimited(ctx, txHash, limit)
}

func (gw *Node) EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := gw.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return nil, err
	}

	return gw.target.EthGetCode(ctx, address, blkParam)
}

func (gw *Node) EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := gw.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return nil, err
	}

	return gw.target.EthGetStorageAt(ctx, address, position, blkParam)
}

func (gw *Node) EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	if err := gw.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	return gw.target.EthGetBalance(ctx, address, blkParam)
}

func (gw *Node) EthChainId(ctx context.Context) (ethtypes.EthUint64, error) {
	if err := gw.limit(ctx, basicRateLimitTokens); err != nil {
		return 0, err
	}

	return gw.target.EthChainId(ctx)
}

func (gw *Node) EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error) {
	if err := gw.limit(ctx, basicRateLimitTokens); err != nil {
		return ethtypes.EthSyncingResult{}, err
	}

	return gw.target.EthSyncing(ctx)
}

func (gw *Node) NetVersion(ctx context.Context) (string, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return "", err
	}

	return gw.target.NetVersion(ctx)
}

func (gw *Node) NetListening(ctx context.Context) (bool, error) {
	if err := gw.limit(ctx, basicRateLimitTokens); err != nil {
		return false, err
	}

	return gw.target.NetListening(ctx)
}

func (gw *Node) EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	return gw.target.EthProtocolVersion(ctx)
}

func (gw *Node) EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	return gw.target.EthGasPrice(ctx)
}

var EthFeeHistoryMaxBlockCount = 128 // this seems to be expensive; todo: figure out what is a good number that works with everything

func (gw *Node) EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error) {
	params, err := jsonrpc.DecodeParams[ethtypes.EthFeeHistoryParams](p)
	if err != nil {
		return ethtypes.EthFeeHistory{}, xerrors.Errorf("decoding params: %w", err)
	}

	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFeeHistory{}, err
	}

	if err := gw.checkBlkParam(ctx, params.NewestBlkNum, params.BlkCount); err != nil {
		return ethtypes.EthFeeHistory{}, err
	}

	if params.BlkCount > ethtypes.EthUint64(EthFeeHistoryMaxBlockCount) {
		return ethtypes.EthFeeHistory{}, xerrors.New("block count too high")
	}

	return gw.target.EthFeeHistory(ctx, p)
}

func (gw *Node) EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}

	return gw.target.EthMaxPriorityFeePerGas(ctx)
}

func (gw *Node) EthEstimateGas(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthUint64, error) {
	// validate params
	_, err := jsonrpc.DecodeParams[ethtypes.EthEstimateGasParams](p)
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("decoding params: %w", err)
	}

	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}

	// todo limit gas? to what?
	return gw.target.EthEstimateGas(ctx, p)
}

func (gw *Node) EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := gw.checkEthBlockParam(ctx, blkParam, 0); err != nil {
		return nil, err
	}

	// todo limit gas? to what?
	return gw.target.EthCall(ctx, tx, blkParam)
}

func (gw *Node) EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthHash{}, err
	}

	return gw.target.EthSendRawTransaction(ctx, rawTx)
}

func (gw *Node) EthGetLogs(ctx context.Context, filter *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if filter.FromBlock != nil {
		if err := gw.checkBlkParam(ctx, *filter.FromBlock, 0); err != nil {
			return nil, err
		}
	}
	if filter.ToBlock != nil {
		if err := gw.checkBlkParam(ctx, *filter.ToBlock, 0); err != nil {
			return nil, err
		}
	}
	if filter.BlockHash != nil {
		if err := gw.checkBlkHash(ctx, *filter.BlockHash); err != nil {
			return nil, err
		}
	}

	return gw.target.EthGetLogs(ctx, filter)
}

func (gw *Node) EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	ft, err := getStatefulTracker(ctx)
	if err != nil {
		return nil, xerrors.Errorf("EthGetFilterChanges not supported: %w", err)
	}
	ft.lk.Lock()
	_, ok := ft.userFilters[id]
	ft.lk.Unlock()

	if !ok {
		return nil, filter.ErrFilterNotFound
	}

	return gw.target.EthGetFilterChanges(ctx, id)
}

func (gw *Node) EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	ft, err := getStatefulTracker(ctx)
	if err != nil {
		return nil, xerrors.Errorf("EthGetFilterLogs not supported: %w", err)
	}
	ft.lk.Lock()
	_, ok := ft.userFilters[id]
	ft.lk.Unlock()

	if !ok {
		return nil, nil
	}

	return gw.target.EthGetFilterLogs(ctx, id)
}

func (gw *Node) EthNewFilter(ctx context.Context, filter *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFilterID{}, err
	}

	return addUserFilterLimited(ctx, "EthNewFilter", func() (ethtypes.EthFilterID, error) {
		return gw.target.EthNewFilter(ctx, filter)
	})
}

func (gw *Node) EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFilterID{}, err
	}

	return addUserFilterLimited(ctx, "EthNewBlockFilter", func() (ethtypes.EthFilterID, error) {
		return gw.target.EthNewBlockFilter(ctx)
	})
}

func (gw *Node) EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthFilterID{}, err
	}

	return addUserFilterLimited(ctx, "EthNewPendingTransactionFilter", func() (ethtypes.EthFilterID, error) {
		return gw.target.EthNewPendingTransactionFilter(ctx)
	})
}

func (gw *Node) EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}

	// check if the filter belongs to this connection
	ft, err := getStatefulTracker(ctx)
	if err != nil {
		return false, xerrors.Errorf("EthUninstallFilter not supported: %w", err)
	}
	ft.lk.Lock()
	defer ft.lk.Unlock()

	if _, ok := ft.userFilters[id]; !ok {
		return false, nil
	}

	ok, err := gw.target.EthUninstallFilter(ctx, id)
	if err != nil {
		return false, err
	}

	delete(ft.userFilters, id)
	return ok, nil
}

func (gw *Node) EthSubscribe(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error) {
	// validate params
	_, err := jsonrpc.DecodeParams[ethtypes.EthSubscribeParams](p)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("decoding params: %w", err)
	}

	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return ethtypes.EthSubscriptionID{}, err
	}

	if gw.subHnd == nil {
		return ethtypes.EthSubscriptionID{}, xerrors.New("subscription support not enabled")
	}

	ethCb, ok := jsonrpc.ExtractReverseClient[api.EthSubscriberMethods](ctx)
	if !ok {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("connection doesn't support callbacks")
	}

	ft, err := getStatefulTracker(ctx)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, xerrors.Errorf("EthSubscribe not supported: %w", err)
	}
	ft.lk.Lock()
	defer ft.lk.Unlock()

	if len(ft.userSubscriptions) >= EthMaxFiltersPerConn {
		return ethtypes.EthSubscriptionID{}, xerrors.New("too many subscriptions")
	}

	sub, err := gw.target.EthSubscribe(ctx, p)
	if err != nil {
		return ethtypes.EthSubscriptionID{}, err
	}

	err = gw.subHnd.AddSub(ctx, sub, func(ctx context.Context, response *ethtypes.EthSubscriptionResponse) error {
		outParam, err := json.Marshal(response)
		if err != nil {
			return err
		}

		return ethCb.EthSubscription(ctx, outParam)
	})
	if err != nil {
		return ethtypes.EthSubscriptionID{}, err
	}

	ft.userSubscriptions[sub] = time.Now()

	return sub, err
}

func (gw *Node) EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}

	// check if the filter belongs to this connection
	ft, err := getStatefulTracker(ctx)
	if err != nil {
		return false, xerrors.Errorf("EthUnsubscribe not supported: %w", err)
	}
	ft.lk.Lock()
	defer ft.lk.Unlock()

	if _, ok := ft.userSubscriptions[id]; !ok {
		return false, nil
	}

	ok, err := gw.target.EthUnsubscribe(ctx, id)
	if err != nil {
		return false, err
	}

	delete(ft.userSubscriptions, id)

	if gw.subHnd != nil {
		gw.subHnd.RemoveSub(id)
	}

	return ok, nil
}

func (gw *Node) Web3ClientVersion(ctx context.Context) (string, error) {
	if err := gw.limit(ctx, basicRateLimitTokens); err != nil {
		return "", err
	}

	return gw.target.Web3ClientVersion(ctx)
}

func (gw *Node) EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := gw.checkBlkParam(ctx, blkNum, 0); err != nil {
		return nil, err
	}

	return gw.target.EthTraceBlock(ctx, blkNum)
}

func (gw *Node) EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if err := gw.checkBlkParam(ctx, blkNum, 0); err != nil {
		return nil, err
	}

	return gw.target.EthTraceReplayBlockTransactions(ctx, blkNum, traceTypes)
}

func (gw *Node) EthTraceTransaction(ctx context.Context, txHash string) ([]*ethtypes.EthTraceTransaction, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	return gw.target.EthTraceTransaction(ctx, txHash)
}

func (gw *Node) EthTraceFilter(ctx context.Context, filter ethtypes.EthTraceFilterCriteria) ([]*ethtypes.EthTraceFilterResult, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}

	if filter.ToBlock != nil {
		if err := gw.checkBlkParam(ctx, *filter.ToBlock, 0); err != nil {
			return nil, err
		}
	}

	if filter.FromBlock != nil {
		if err := gw.checkBlkParam(ctx, *filter.FromBlock, 0); err != nil {
			return nil, err
		}
	}

	return gw.target.EthTraceFilter(ctx, filter)
}

var EthMaxFiltersPerConn = 16 // todo make this configurable

func addUserFilterLimited(ctx context.Context, callName string, cb func() (ethtypes.EthFilterID, error)) (ethtypes.EthFilterID, error) {
	ft, err := getStatefulTracker(ctx)
	if err != nil {
		return ethtypes.EthFilterID{}, xerrors.Errorf("%s not supported: %w", callName, err)
	}
	ft.lk.Lock()
	defer ft.lk.Unlock()

	if len(ft.userFilters) >= EthMaxFiltersPerConn {
		return ethtypes.EthFilterID{}, xerrors.New("too many filters")
	}

	id, err := cb()
	if err != nil {
		return id, err
	}

	ft.userFilters[id] = time.Now()

	return id, nil
}

func getStatefulTracker(ctx context.Context) (*statefulCallTracker, error) {
	if jsonrpc.GetConnectionType(ctx) != jsonrpc.ConnectionTypeWS {
		return nil, xerrors.New("stateful tracking is only available for websockets connections")
	}

	if ct, ok := ctx.Value(statefulCallTrackerKey).(*statefulCallTracker); !ok {
		return nil, xerrors.New("stateful tracking is not available for this call")
	} else {
		return ct, nil
	}
}

type statefulCallTracker struct {
	lk sync.Mutex

	userFilters       map[ethtypes.EthFilterID]time.Time
	userSubscriptions map[ethtypes.EthSubscriptionID]time.Time
}

// called per request (ws connection)
func newStatefulCallTracker() *statefulCallTracker {
	return &statefulCallTracker{
		userFilters:       make(map[ethtypes.EthFilterID]time.Time),
		userSubscriptions: make(map[ethtypes.EthSubscriptionID]time.Time),
	}
}
