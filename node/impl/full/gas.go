package full

import (
	"context"
	"math"
	"math/rand"
	"sort"

	"github.com/filecoin-project/lotus/chain/actors/builtin"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

type GasModuleAPI interface {
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error)
}

// GasModule provides a default implementation of GasModuleAPI.
// It can be swapped out with another implementation through Dependency
// Injection (for example with a thin RPC client).
type GasModule struct {
	fx.In
	Stmgr *stmgr.StateManager
	Chain *store.ChainStore
	Mpool *messagepool.MessagePool
}

var _ GasModuleAPI = (*GasModule)(nil)

type GasAPI struct {
	fx.In

	GasModuleAPI

	Stmgr *stmgr.StateManager
	Chain *store.ChainStore
	Mpool *messagepool.MessagePool
}

const MinGasPremium = 100e3
const MaxSpendOnFeeDenom = 100

func (a *GasAPI) GasEstimateFeeCap(
	ctx context.Context,
	msg *types.Message,
	maxqueueblks int64,
	tsk types.TipSetKey,
) (types.BigInt, error) {
	return gasEstimateFeeCap(a.Chain, msg, maxqueueblks)
}
func (m *GasModule) GasEstimateFeeCap(
	ctx context.Context,
	msg *types.Message,
	maxqueueblks int64,
	tsk types.TipSetKey,
) (types.BigInt, error) {
	return gasEstimateFeeCap(m.Chain, msg, maxqueueblks)
}
func gasEstimateFeeCap(cstore *store.ChainStore, msg *types.Message, maxqueueblks int64) (types.BigInt, error) {
	ts := cstore.GetHeaviestTipSet()

	parentBaseFee := ts.Blocks()[0].ParentBaseFee
	increaseFactor := math.Pow(1.+1./float64(build.BaseFeeMaxChangeDenom), float64(maxqueueblks))

	feeInFuture := types.BigMul(parentBaseFee, types.NewInt(uint64(increaseFactor*(1<<8))))
	out := types.BigDiv(feeInFuture, types.NewInt(1<<8))

	if msg.GasPremium != types.EmptyInt {
		out = types.BigAdd(out, msg.GasPremium)
	}

	return out, nil
}

type gasMeta struct {
	price big.Int
	limit int64
}

func medianGasPremium(prices []gasMeta, blocks int) abi.TokenAmount {
	sort.Slice(prices, func(i, j int) bool {
		// sort desc by price
		return prices[i].price.GreaterThan(prices[j].price)
	})

	at := build.BlockGasTarget * int64(blocks) / 2
	prev1, prev2 := big.Zero(), big.Zero()
	for _, price := range prices {
		prev1, prev2 = price.price, prev1
		at -= price.limit
		if at < 0 {
			break
		}
	}

	premium := prev1
	if prev2.Sign() != 0 {
		premium = big.Div(types.BigAdd(prev1, prev2), types.NewInt(2))
	}

	return premium
}

func (a *GasAPI) GasEstimateGasPremium(
	ctx context.Context,
	nblocksincl uint64,
	sender address.Address,
	gaslimit int64,
	_ types.TipSetKey,
) (types.BigInt, error) {
	return gasEstimateGasPremium(a.Chain, nblocksincl)
}
func (m *GasModule) GasEstimateGasPremium(
	ctx context.Context,
	nblocksincl uint64,
	sender address.Address,
	gaslimit int64,
	_ types.TipSetKey,
) (types.BigInt, error) {
	return gasEstimateGasPremium(m.Chain, nblocksincl)
}
func gasEstimateGasPremium(cstore *store.ChainStore, nblocksincl uint64) (types.BigInt, error) {
	if nblocksincl == 0 {
		nblocksincl = 1
	}

	var prices []gasMeta
	var blocks int

	ts := cstore.GetHeaviestTipSet()
	for i := uint64(0); i < nblocksincl*2; i++ {
		if ts.Height() == 0 {
			break // genesis
		}

		pts, err := cstore.LoadTipSet(ts.Parents())
		if err != nil {
			return types.BigInt{}, err
		}

		blocks += len(pts.Blocks())

		msgs, err := cstore.MessagesForTipset(pts)
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

	premium := medianGasPremium(prices, blocks)

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
	return gasEstimateGasLimit(ctx, a.Chain, a.Stmgr, a.Mpool, msgIn)
}
func (m *GasModule) GasEstimateGasLimit(ctx context.Context, msgIn *types.Message, _ types.TipSetKey) (int64, error) {
	return gasEstimateGasLimit(ctx, m.Chain, m.Stmgr, m.Mpool, msgIn)
}
func gasEstimateGasLimit(
	ctx context.Context,
	cstore *store.ChainStore,
	smgr *stmgr.StateManager,
	mpool *messagepool.MessagePool,
	msgIn *types.Message,
) (int64, error) {
	msg := *msgIn
	msg.GasLimit = build.BlockGasLimit
	msg.GasFeeCap = types.NewInt(uint64(build.MinimumBaseFee) + 1)
	msg.GasPremium = types.NewInt(1)

	currTs := cstore.GetHeaviestTipSet()
	fromA, err := smgr.ResolveToKeyAddress(ctx, msgIn.From, currTs)
	if err != nil {
		return -1, xerrors.Errorf("getting key address: %w", err)
	}

	pending, ts := mpool.PendingFor(fromA)
	priorMsgs := make([]types.ChainMsg, 0, len(pending))
	for _, m := range pending {
		priorMsgs = append(priorMsgs, m)
	}

	// Try calling until we find a height with no migration.
	var res *api.InvocResult
	for {
		res, err = smgr.CallWithGas(ctx, &msg, priorMsgs, ts)
		if err != stmgr.ErrExpensiveFork {
			break
		}
		ts, err = cstore.GetTipSetFromKey(ts.Parents())
		if err != nil {
			return -1, xerrors.Errorf("getting parent tipset: %w", err)
		}
	}
	if err != nil {
		return -1, xerrors.Errorf("CallWithGas failed: %w", err)
	}
	if res.MsgRct.ExitCode != exitcode.Ok {
		return -1, xerrors.Errorf("message execution failed: exit %s, reason: %s", res.MsgRct.ExitCode, res.Error)
	}

	// Special case for PaymentChannel collect, which is deleting actor
	st, err := smgr.ParentState(ts)
	if err != nil {
		_ = err
		// somewhat ignore it as it can happen and we just want to detect
		// an existing PaymentChannel actor
		return res.MsgRct.GasUsed, nil
	}
	act, err := st.GetActor(msg.To)
	if err != nil {
		_ = err
		// somewhat ignore it as it can happen and we just want to detect
		// an existing PaymentChannel actor
		return res.MsgRct.GasUsed, nil
	}

	if !builtin.IsPaymentChannelActor(act.Code) {
		return res.MsgRct.GasUsed, nil
	}
	if msgIn.Method != builtin0.MethodsPaych.Collect {
		return res.MsgRct.GasUsed, nil
	}

	// return GasUsed without the refund for DestoryActor
	return res.MsgRct.GasUsed + 76e3, nil
}

func (m *GasModule) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, _ types.TipSetKey) (*types.Message, error) {
	if msg.GasLimit == 0 {
		gasLimit, err := m.GasEstimateGasLimit(ctx, msg, types.TipSetKey{})
		if err != nil {
			return nil, xerrors.Errorf("estimating gas used: %w", err)
		}
		msg.GasLimit = int64(float64(gasLimit) * m.Mpool.GetConfig().GasLimitOverestimation)
	}

	if msg.GasPremium == types.EmptyInt || types.BigCmp(msg.GasPremium, types.NewInt(0)) == 0 {
		gasPremium, err := m.GasEstimateGasPremium(ctx, 2, msg.From, msg.GasLimit, types.TipSetKey{})
		if err != nil {
			return nil, xerrors.Errorf("estimating gas price: %w", err)
		}
		msg.GasPremium = gasPremium
	}

	if msg.GasFeeCap == types.EmptyInt || types.BigCmp(msg.GasFeeCap, types.NewInt(0)) == 0 {
		feeCap, err := m.GasEstimateFeeCap(ctx, msg, 20, types.EmptyTSK)
		if err != nil {
			return nil, xerrors.Errorf("estimating fee cap: %w", err)
		}
		msg.GasFeeCap = feeCap
	}

	messagepool.CapGasFee(msg, spec.Get().MaxFee)

	return msg, nil
}
