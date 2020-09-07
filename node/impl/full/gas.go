package full

import (
	"context"
	"math"
	"math/rand"
	"sort"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/specs-actors/actors/builtin"

	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

type GasAPI struct {
	fx.In
	Stmgr *stmgr.StateManager
	Chain *store.ChainStore
	Mpool *messagepool.MessagePool
}

const MinGasPremium = 100e3
const MaxSpendOnFeeDenom = 100

func (a *GasAPI) GasEstimateFeeCap(ctx context.Context, msg *types.Message, maxqueueblks int64,
	tsk types.TipSetKey) (types.BigInt, error) {
	ts := a.Chain.GetHeaviestTipSet()

	parentBaseFee := ts.Blocks()[0].ParentBaseFee
	increaseFactor := math.Pow(1.+1./float64(build.BaseFeeMaxChangeDenom), float64(maxqueueblks))

	feeInFuture := types.BigMul(parentBaseFee, types.NewInt(uint64(increaseFactor*(1<<8))))
	out := types.BigDiv(feeInFuture, types.NewInt(1<<8))

	if msg.GasPremium != types.EmptyInt {
		out = types.BigAdd(out, msg.GasPremium)
	}

	return out, nil
}

func (a *GasAPI) GasEstimateGasPremium(ctx context.Context, nblocksincl uint64,
	sender address.Address, gaslimit int64, _ types.TipSetKey) (types.BigInt, error) {

	if nblocksincl == 0 {
		nblocksincl = 1
	}

	type gasMeta struct {
		price big.Int
		limit int64
	}

	var prices []gasMeta
	var blocks int

	ts := a.Chain.GetHeaviestTipSet()
	for i := uint64(0); i < nblocksincl*2; i++ {
		if ts.Height() == 0 {
			break // genesis
		}

		pts, err := a.Chain.LoadTipSet(ts.Parents())
		if err != nil {
			return types.BigInt{}, err
		}

		blocks += len(pts.Blocks())

		msgs, err := a.Chain.MessagesForTipset(pts)
		if err != nil {
			return types.BigInt{}, xerrors.Errorf("loading messages: %w", err)
		}
		for _, msg := range msgs {
			prices = append(prices, gasMeta{
				price: msg.VMMessage().GasPremium,
				limit: msg.VMMessage().GasLimit,
			})
		}

		ts = pts
	}

	sort.Slice(prices, func(i, j int) bool {
		// sort desc by price
		return prices[i].price.GreaterThan(prices[j].price)
	})

	at := build.BlockGasTarget * int64(blocks) / 2
	prev1, prev2 := big.Zero(), big.Zero()
	for _, price := range prices {
		prev1, prev2 = price.price, prev1
		at -= price.limit
		if at > 0 {
			continue
		}
	}

	premium := prev1
	if prev2.Sign() != 0 {
		premium = big.Div(types.BigAdd(prev1, prev2), types.NewInt(2))
	}

	if types.BigCmp(premium, types.NewInt(MinGasPremium)) < 0 {
		switch nblocksincl {
		case 1:
			premium = types.NewInt(2 * MinGasPremium)
		case 2:
			premium = types.NewInt(1.5 * MinGasPremium)
		default:
			premium = types.NewInt(MinGasPremium)
		}
	}

	// add some noise to normalize behaviour of message selection
	const precision = 32
	// mean 1, stddev 0.005 => 95% within +-1%
	noise := 1 + rand.NormFloat64()*0.005
	premium = types.BigMul(premium, types.NewInt(uint64(noise*(1<<precision))+1))
	premium = types.BigDiv(premium, types.NewInt(1<<precision))
	return premium, nil
}

func (a *GasAPI) GasEstimateGasLimit(ctx context.Context, msgIn *types.Message, _ types.TipSetKey) (int64, error) {

	msg := *msgIn
	msg.GasLimit = build.BlockGasLimit
	msg.GasFeeCap = types.NewInt(uint64(build.MinimumBaseFee) + 1)
	msg.GasPremium = types.NewInt(1)

	currTs := a.Chain.GetHeaviestTipSet()
	fromA, err := a.Stmgr.ResolveToKeyAddress(ctx, msgIn.From, currTs)
	if err != nil {
		return -1, xerrors.Errorf("getting key address: %w", err)
	}

	pending, ts := a.Mpool.PendingFor(fromA)
	priorMsgs := make([]types.ChainMsg, 0, len(pending))
	for _, m := range pending {
		priorMsgs = append(priorMsgs, m)
	}

	res, err := a.Stmgr.CallWithGas(ctx, &msg, priorMsgs, ts)
	if err != nil {
		return -1, xerrors.Errorf("CallWithGas failed: %w", err)
	}
	if res.MsgRct.ExitCode != exitcode.Ok {
		return -1, xerrors.Errorf("message execution failed: exit %s, reason: %s", res.MsgRct.ExitCode, res.Error)
	}

	// Special case for PaymentChannel collect, which is deleting actor
	var act types.Actor
	err = a.Stmgr.WithParentState(ts, a.Stmgr.WithActor(msg.To, stmgr.GetActor(&act)))
	if err != nil {
		_ = err
		// somewhat ignore it as it can happen and we just want to detect
		// an existing PaymentChannel actor
		return res.MsgRct.GasUsed, nil
	}

	if !act.Code.Equals(builtin.PaymentChannelActorCodeID) {
		return res.MsgRct.GasUsed, nil
	}
	if msgIn.Method != builtin.MethodsPaych.Collect {
		return res.MsgRct.GasUsed, nil
	}

	// return GasUsed without the refund for DestoryActor
	return res.MsgRct.GasUsed + 76e3, nil
}

func (a *GasAPI) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, _ types.TipSetKey) (*types.Message, error) {
	if msg.GasLimit == 0 {
		gasLimit, err := a.GasEstimateGasLimit(ctx, msg, types.TipSetKey{})
		if err != nil {
			return nil, xerrors.Errorf("estimating gas used: %w", err)
		}
		msg.GasLimit = int64(float64(gasLimit) * a.Mpool.GetConfig().GasLimitOverestimation)
	}

	if msg.GasPremium == types.EmptyInt || types.BigCmp(msg.GasPremium, types.NewInt(0)) == 0 {
		gasPremium, err := a.GasEstimateGasPremium(ctx, 2, msg.From, msg.GasLimit, types.TipSetKey{})
		if err != nil {
			return nil, xerrors.Errorf("estimating gas price: %w", err)
		}
		msg.GasPremium = gasPremium
	}

	if msg.GasFeeCap == types.EmptyInt || types.BigCmp(msg.GasFeeCap, types.NewInt(0)) == 0 {
		feeCap, err := a.GasEstimateFeeCap(ctx, msg, 10, types.EmptyTSK)
		if err != nil {
			return nil, xerrors.Errorf("estimating fee cap: %w", err)
		}
		msg.GasFeeCap = feeCap
	}

	capGasFee(msg, spec.Get().MaxFee)

	return msg, nil
}

func capGasFee(msg *types.Message, maxFee abi.TokenAmount) {
	if maxFee.Equals(big.Zero()) {
		maxFee = types.NewInt(build.FilecoinPrecision / 10)
	}

	gl := types.NewInt(uint64(msg.GasLimit))
	totalFee := types.BigMul(msg.GasFeeCap, gl)

	if totalFee.LessThanEqual(maxFee) {
		return
	}

	msg.GasFeeCap = big.Div(maxFee, gl)
	msg.GasPremium = big.Min(msg.GasFeeCap, msg.GasPremium) // cap premium at FeeCap
}
