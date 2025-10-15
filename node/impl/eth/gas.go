package eth

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sort"

	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/impl/gasutils"
)

const maxEthFeeHistoryRewardPercentiles = 100

var (
	_ EthGasAPI = (*ethGas)(nil)
	_ EthGasAPI = (*EthGasDisabled)(nil)
)

var minGasPremium = ethtypes.EthBigInt(types.NewInt(gasutils.MinGasPremium))

type ethGas struct {
	chainStore   ChainStore
	stateManager StateManager
	messagePool  MessagePool
	gasApi       GasAPI

	tipsetResolver TipSetResolver
}

func NewEthGasAPI(
	chainStore ChainStore,
	stateManager StateManager,
	messagePool MessagePool,
	gasApi GasAPI,
	tipsetResolver TipSetResolver,
) EthGasAPI {
	return &ethGas{
		chainStore:     chainStore,
		stateManager:   stateManager,
		messagePool:    messagePool,
		gasApi:         gasApi,
		tipsetResolver: tipsetResolver,
	}
}

func (e *ethGas) EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error) {
	// According to Geth's implementation, eth_gasPrice should return base + tip
	// Ref: https://github.com/ethereum/pm/issues/328#issuecomment-853234014

	ts := e.chainStore.GetHeaviestTipSet()
	baseFee := ts.Blocks()[0].ParentBaseFee

	premium, err := e.EthMaxPriorityFeePerGas(ctx)
	if err != nil {
		return ethtypes.EthBigInt(big.Zero()), nil
	}

	gasPrice := big.Add(baseFee, big.Int(premium))
	return ethtypes.EthBigInt(gasPrice), nil
}

func (e *ethGas) EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error) {
	params, err := jsonrpc.DecodeParams[ethtypes.EthFeeHistoryParams](p)
	if err != nil {
		return ethtypes.EthFeeHistory{}, xerrors.Errorf("decoding params: %w", err)
	}
	if params.BlkCount > 1024 {
		return ethtypes.EthFeeHistory{}, xerrors.New("block count should be smaller than 1024")
	}
	rewardPercentiles := make([]float64, 0)
	if params.RewardPercentiles != nil {
		if len(*params.RewardPercentiles) > maxEthFeeHistoryRewardPercentiles {
			return ethtypes.EthFeeHistory{}, xerrors.New("length of the reward percentile array cannot be greater than 100")
		}
		rewardPercentiles = append(rewardPercentiles, *params.RewardPercentiles...)
	}
	for i, rp := range rewardPercentiles {
		if rp < 0 || rp > 100 {
			return ethtypes.EthFeeHistory{}, xerrors.Errorf("invalid reward percentile: %f should be between 0 and 100", rp)
		}
		if i > 0 && rp < rewardPercentiles[i-1] {
			return ethtypes.EthFeeHistory{}, xerrors.Errorf("invalid reward percentile: %f should be larger than %f", rp, rewardPercentiles[i-1])
		}
	}

	ts, err := e.tipsetResolver.GetTipsetByBlockNumber(ctx, params.NewestBlkNum, false)
	if err != nil {
		return ethtypes.EthFeeHistory{}, err // don't wrap, to preserve ErrNullRound
	}

	var (
		basefee         = ts.Blocks()[0].ParentBaseFee
		oldestBlkHeight = uint64(1)

		// NOTE: baseFeePerGas should include the next block after the newest of the returned range,
		//  because the next base fee can be inferred from the messages in the newest block.
		//  However, this is NOT the case in Filecoin due to deferred execution, so the best
		//  we can do is duplicate the last value.
		baseFeeArray      = []ethtypes.EthBigInt{ethtypes.EthBigInt(basefee)}
		rewardsArray      = make([][]ethtypes.EthBigInt, 0)
		gasUsedRatioArray = []float64{}
		blocksIncluded    int
	)

	for blocksIncluded < int(params.BlkCount) && ts.Height() > 0 {
		basefee = ts.Blocks()[0].ParentBaseFee
		_, msgs, rcpts, err := executeTipset(ctx, ts, e.chainStore, e.stateManager)
		if err != nil {
			return ethtypes.EthFeeHistory{}, xerrors.Errorf("failed to retrieve messages and receipts for height %d: %w", ts.Height(), err)
		}

		txGasRewards := gasRewardSorter{}
		for i, msg := range msgs {
			effectivePremium := msg.VMMessage().EffectiveGasPremium(basefee)
			txGasRewards = append(txGasRewards, gasRewardTuple{
				premium: effectivePremium,
				gasUsed: rcpts[i].GasUsed,
			})
		}

		rewards, totalGasUsed := calculateRewardsAndGasUsed(rewardPercentiles, txGasRewards)
		maxGas := buildconstants.BlockGasLimit * int64(len(ts.Blocks()))

		// arrays should be reversed at the end
		baseFeeArray = append(baseFeeArray, ethtypes.EthBigInt(basefee))
		gasUsedRatioArray = append(gasUsedRatioArray, float64(totalGasUsed)/float64(maxGas))
		rewardsArray = append(rewardsArray, rewards)
		oldestBlkHeight = uint64(ts.Height())
		blocksIncluded++

		parentTsKey := ts.Parents()
		ts, err = e.chainStore.LoadTipSet(ctx, parentTsKey)
		if err != nil {
			return ethtypes.EthFeeHistory{}, xerrors.Errorf("cannot load tipset key: %v", parentTsKey)
		}
	}

	// Reverse the arrays; we collected them newest to oldest; the client expects oldest to newest.
	for i, j := 0, len(baseFeeArray)-1; i < j; i, j = i+1, j-1 {
		baseFeeArray[i], baseFeeArray[j] = baseFeeArray[j], baseFeeArray[i]
	}
	for i, j := 0, len(gasUsedRatioArray)-1; i < j; i, j = i+1, j-1 {
		gasUsedRatioArray[i], gasUsedRatioArray[j] = gasUsedRatioArray[j], gasUsedRatioArray[i]
	}
	for i, j := 0, len(rewardsArray)-1; i < j; i, j = i+1, j-1 {
		rewardsArray[i], rewardsArray[j] = rewardsArray[j], rewardsArray[i]
	}

	ret := ethtypes.EthFeeHistory{
		OldestBlock:   ethtypes.EthUint64(oldestBlkHeight),
		BaseFeePerGas: baseFeeArray,
		GasUsedRatio:  gasUsedRatioArray,
	}
	if params.RewardPercentiles != nil {
		ret.Reward = &rewardsArray
	}
	return ret, nil
}

func (e *ethGas) EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error) {
	gasPremium, err := e.gasApi.GasEstimateGasPremium(ctx, 0, builtinactors.SystemActorAddr, 10000, types.EmptyTSK)
	if err != nil {
		return ethtypes.EthBigInt(big.Zero()), err
	}
	return ethtypes.EthBigInt(gasPremium), nil
}

func (e *ethGas) EthEstimateGas(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthUint64, error) {
	params, err := jsonrpc.DecodeParams[ethtypes.EthEstimateGasParams](p)
	if err != nil {
		return ethtypes.EthUint64(0), xerrors.Errorf("decoding params: %w", err)
	}

	msg, err := params.Tx.ToFilecoinMessage()
	if err != nil {
		return ethtypes.EthUint64(0), err
	}

	// Set the gas limit to the zero sentinel value, which makes
	// gas estimation actually run.
	msg.GasLimit = 0

	var ts *types.TipSet
	if params.BlkParam == nil {
		ts = e.chainStore.GetHeaviestTipSet()
	} else {
		ts, err = e.tipsetResolver.GetTipsetByBlockNumberOrHash(ctx, *params.BlkParam)
		if err != nil {
			return ethtypes.EthUint64(0), err
		}
	}

	gassedMsg, err := e.gasApi.GasEstimateMessageGas(ctx, msg, nil, ts.Key())
	if err != nil {
		// On failure, GasEstimateMessageGas doesn't actually return the invocation result,
		// it just returns an error. That means we can't get the revert reason.
		//
		// So we re-execute the message with EthCall (well, applyMessage which contains the
		// guts of EthCall). This will give us an ethereum specific error with revert
		// information.
		msg.GasLimit = buildconstants.BlockGasLimit
		if _, err2 := e.applyMessage(ctx, msg, ts.Key()); err2 != nil {
			// If err2 is an ExecutionRevertedError, return it
			var ed *api.ErrExecutionReverted
			if errors.As(err2, &ed) {
				return ethtypes.EthUint64(0), err2
			}

			// Otherwise, return the error from applyMessage with failed to estimate gas
			err = err2
		}

		return ethtypes.EthUint64(0), xerrors.Errorf("failed to estimate gas: %w", err)
	}

	expectedGas, err := ethGasSearch(ctx, e.chainStore, e.stateManager, e.messagePool, gassedMsg, ts)
	if err != nil {
		return 0, xerrors.Errorf("gas search failed: %w", err)
	}

	return ethtypes.EthUint64(expectedGas), nil
}

func (e *ethGas) EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	msg, err := tx.ToFilecoinMessage()
	if err != nil {
		return nil, xerrors.Errorf("failed to convert ethcall to filecoin message: %w", err)
	}

	ts, err := e.tipsetResolver.GetTipsetByBlockNumberOrHash(ctx, blkParam)
	if err != nil {
		return nil, err // don't wrap, to preserve ErrNullRound
	}

	invokeResult, err := e.applyMessage(ctx, msg, ts.Key())
	if err != nil {
		return nil, err
	}

	if msg.To == builtintypes.EthereumAddressManagerActorAddr {
		return ethtypes.EthBytes{}, nil
	} else if len(invokeResult.MsgRct.Return) > 0 {
		return cbg.ReadByteArray(bytes.NewReader(invokeResult.MsgRct.Return), uint64(len(invokeResult.MsgRct.Return)))
	}

	return ethtypes.EthBytes{}, nil
}

func (e *ethGas) applyMessage(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (res *api.InvocResult, err error) {
	ts, err := e.chainStore.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("cannot get tipset: %w", err)
	}

	if ts.Height() > 0 {
		pts, err := e.chainStore.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("failed to find a non-forking epoch: %w", err)
		}
		// Check for expensive forks from the parents to the tipset, including nil tipsets
		if e.stateManager.HasExpensiveForkBetween(pts.Height(), ts.Height()+1) {
			return nil, stmgr.ErrExpensiveFork
		}
	}

	st, _, err := e.stateManager.TipSetState(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("cannot get tipset state: %w", err)
	}
	res, err = e.stateManager.ApplyOnStateWithGas(ctx, st, msg, ts)
	if err != nil {
		return nil, xerrors.Errorf("ApplyWithGasOnState failed: %w", err)
	}

	if res.MsgRct.ExitCode.IsError() {
		return nil, api.NewErrExecutionRevertedFromResult(res)
	}

	return res, nil
}

// ethGasSearch executes a message for gas estimation using the previously estimated gas.
// If the message fails due to an out of gas error then a gas search is performed.
// See gasSearch.
func ethGasSearch(
	ctx context.Context,
	chainStore ChainStore,
	stateManager StateManager,
	messagePool MessagePool,
	msgIn *types.Message,
	ts *types.TipSet,
) (int64, error) {
	msg := *msgIn
	currTs := ts

	res, priorMsgs, ts, err := gasutils.GasEstimateCallWithGas(ctx, chainStore, stateManager, messagePool, &msg, currTs)
	if err != nil {
		return -1, xerrors.Errorf("gas estimation failed: %w", err)
	}

	if res.MsgRct.ExitCode.IsSuccess() {
		return msg.GasLimit, nil
	}

	if traceContainsExitCode(res.ExecutionTrace, exitcode.SysErrOutOfGas) {
		ret, err := gasSearch(ctx, stateManager, &msg, priorMsgs, ts)
		if err != nil {
			return -1, xerrors.Errorf("gas estimation search failed: %w", err)
		}

		ret = int64(float64(ret) * messagePool.GetConfig().GasLimitOverestimation)
		return ret, nil
	}

	return -1, api.NewErrExecutionRevertedFromResult(res)
}

func traceContainsExitCode(et types.ExecutionTrace, ex exitcode.ExitCode) bool {
	if et.MsgRct.ExitCode == ex {
		return true
	}

	for _, et := range et.Subcalls {
		if traceContainsExitCode(et, ex) {
			return true
		}
	}

	return false
}

// gasSearch does an exponential search to find a gas value to execute the
// message with. It first finds a high gas limit that allows the message to execute
// by doubling the previous gas limit until it succeeds then does a binary
// search till it gets within a range of 1%
func gasSearch(
	ctx context.Context,
	stateManager StateManager,
	msgIn *types.Message,
	priorMsgs []types.ChainMsg,
	ts *types.TipSet,
) (int64, error) {
	msg := *msgIn

	high := msg.GasLimit
	low := msg.GasLimit

	applyTsMessages := true
	if os.Getenv("LOTUS_SKIP_APPLY_TS_MESSAGE_CALL_WITH_GAS") == "1" {
		applyTsMessages = false
	}

	canSucceed := func(limit int64) (bool, error) {
		msg.GasLimit = limit

		res, err := stateManager.CallWithGas(ctx, &msg, priorMsgs, ts, applyTsMessages)
		if err != nil {
			return false, xerrors.Errorf("CallWithGas failed: %w", err)
		}

		if res.MsgRct.ExitCode.IsSuccess() {
			return true, nil
		}

		return false, nil
	}

	for {
		ok, err := canSucceed(high)
		if err != nil {
			return -1, xerrors.Errorf("searching for high gas limit failed: %w", err)
		}
		if ok {
			break
		}

		low = high
		high = high * 2

		if high > buildconstants.BlockGasLimit {
			high = buildconstants.BlockGasLimit
			break
		}
	}

	checkThreshold := high / 100
	for (high - low) > checkThreshold {
		median := (low + high) / 2
		ok, err := canSucceed(median)
		if err != nil {
			return -1, xerrors.Errorf("searching for optimal gas limit failed: %w", err)
		}

		if ok {
			high = median
		} else {
			low = median
		}

		checkThreshold = median / 100
	}

	return high, nil
}

func calculateRewardsAndGasUsed(rewardPercentiles []float64, txGasRewards gasRewardSorter) ([]ethtypes.EthBigInt, int64) {
	var gasUsedTotal int64
	for _, tx := range txGasRewards {
		gasUsedTotal += tx.gasUsed
	}

	rewards := make([]ethtypes.EthBigInt, len(rewardPercentiles))
	for i := range rewards {
		rewards[i] = minGasPremium
	}

	if len(txGasRewards) == 0 {
		return rewards, gasUsedTotal
	}

	sort.Stable(txGasRewards)

	var idx int
	var sum int64
	for i, percentile := range rewardPercentiles {
		threshold := int64(float64(gasUsedTotal) * percentile / 100)
		for sum < threshold && idx < len(txGasRewards)-1 {
			sum += txGasRewards[idx].gasUsed
			idx++
		}
		rewards[i] = ethtypes.EthBigInt(txGasRewards[idx].premium)
	}

	return rewards, gasUsedTotal
}

type gasRewardTuple struct {
	gasUsed int64
	premium abi.TokenAmount
}

// sorted in ascending order
type gasRewardSorter []gasRewardTuple

func (g gasRewardSorter) Len() int { return len(g) }
func (g gasRewardSorter) Swap(i, j int) {
	g[i], g[j] = g[j], g[i]
}
func (g gasRewardSorter) Less(i, j int) bool {
	return g[i].premium.Int.Cmp(g[j].premium.Int) == -1
}

type EthGasDisabled struct{}

func (EthGasDisabled) EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error) {
	return ethtypes.EthBigInt{}, ErrModuleDisabled
}
func (EthGasDisabled) EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error) {
	return ethtypes.EthFeeHistory{}, ErrModuleDisabled
}
func (EthGasDisabled) EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error) {
	return ethtypes.EthBigInt{}, ErrModuleDisabled
}
func (EthGasDisabled) EthEstimateGas(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthUint64, error) {
	return ethtypes.EthUint64(0), ErrModuleDisabled
}
func (EthGasDisabled) EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	return nil, ErrModuleDisabled
}
