package full

import (
	"context"
	"math"
	"sort"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

type GasAPI struct {
	fx.In
	Stmgr *stmgr.StateManager
	Chain *store.ChainStore
	Mpool *messagepool.MessagePool
}

const MinGasPremium = 10e3
const BaseFeeEstimNBlocks = 20

func (a *GasAPI) GasEstimateFeeCap(ctx context.Context, maxqueueblks int64,
	tsk types.TipSetKey) (types.BigInt, error) {
	ts := a.Chain.GetHeaviestTipSet()

	parentBaseFee := ts.Blocks()[0].ParentBaseFee
	increaseFactor := math.Pow(1+float64(1/build.BaseFeeMaxChangeDenom), BaseFeeEstimNBlocks)

	out := types.BigMul(parentBaseFee, types.NewInt(uint64(increaseFactor*(1<<8))))
	out = types.BigDiv(out, types.NewInt(1<<8))
	return out, nil
}

func (a *GasAPI) GasEsitmateGasPremium(ctx context.Context, nblocksincl uint64,
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
		if len(ts.Parents().Cids()) == 0 {
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

	// todo: account for how full blocks are

	at := build.BlockGasTarget * int64(blocks) / 2
	prev := big.Zero()

	for _, price := range prices {
		at -= price.limit
		if at > 0 {
			prev = price.price
			continue
		}

		if prev.Equals(big.Zero()) {
			return types.BigAdd(price.price, big.NewInt(1)), nil
		}

		return types.BigAdd(big.Div(types.BigAdd(price.price, prev), types.NewInt(2)), big.NewInt(1)), nil
	}

	switch nblocksincl {
	case 1:
		return types.NewInt(2 * MinGasPremium), nil
	case 2:
		return types.NewInt(1.5 * MinGasPremium), nil
	default:
		return types.NewInt(MinGasPremium), nil
	}
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

	return res.MsgRct.GasUsed, nil
}
