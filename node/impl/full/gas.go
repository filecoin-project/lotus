package full

import (
	"context"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/gasutils"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type GasModuleAPI interface {
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error)
}

var _ GasModuleAPI = *new(api.FullNode)

// GasModule provides a default implementation of GasModuleAPI.
// It can be swapped out with another implementation through Dependency
// Injection (for example with a thin RPC client).
type GasModule struct {
	fx.In
	Stmgr     *stmgr.StateManager
	Chain     *store.ChainStore
	Mpool     *messagepool.MessagePool
	GetMaxFee dtypes.DefaultMaxFeeFunc

	PriceCache *gasutils.GasPriceCache
}

var _ GasModuleAPI = (*GasModule)(nil)

type GasAPI struct {
	fx.In

	GasModuleAPI

	Stmgr *stmgr.StateManager
	Chain *store.ChainStore
	Mpool *messagepool.MessagePool

	PriceCache *gasutils.GasPriceCache
}

func (a *GasAPI) GasEstimateFeeCap(
	ctx context.Context,
	msg *types.Message,
	maxqueueblks int64,
	tsk types.TipSetKey,
) (types.BigInt, error) {
	return gasutils.GasEstimateFeeCap(ctx, a.Chain, msg, maxqueueblks, tsk)
}

func (m *GasModule) GasEstimateFeeCap(
	ctx context.Context,
	msg *types.Message,
	maxqueueblks int64,
	tsk types.TipSetKey,
) (types.BigInt, error) {
	return gasutils.GasEstimateFeeCap(ctx, m.Chain, msg, maxqueueblks, tsk)
}

func (a *GasAPI) GasEstimateGasPremium(
	ctx context.Context,
	nblocksincl uint64,
	sender address.Address,
	gaslimit int64,
	tsk types.TipSetKey,
) (types.BigInt, error) {
	return gasutils.GasEstimateGasPremium(ctx, a.Chain, a.PriceCache, nblocksincl, tsk)
}

func (m *GasModule) GasEstimateGasPremium(
	ctx context.Context,
	nblocksincl uint64,
	sender address.Address,
	gaslimit int64,
	tsk types.TipSetKey,
) (types.BigInt, error) {
	return gasutils.GasEstimateGasPremium(ctx, m.Chain, m.PriceCache, nblocksincl, tsk)
}

func (a *GasAPI) GasEstimateGasLimit(ctx context.Context, msgIn *types.Message, tsk types.TipSetKey) (int64, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return -1, xerrors.Errorf("getting tipset: %w", err)
	}
	return gasutils.GasEstimateGasLimit(ctx, a.Chain, a.Stmgr, a.Mpool, msgIn, ts)
}

func (m *GasModule) GasEstimateGasLimit(ctx context.Context, msgIn *types.Message, tsk types.TipSetKey) (int64, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return -1, xerrors.Errorf("getting tipset: %w", err)
	}
	return gasutils.GasEstimateGasLimit(ctx, m.Chain, m.Stmgr, m.Mpool, msgIn, ts)
}

func (m *GasModule) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, ts types.TipSetKey) (*types.Message, error) {
	if msg.GasLimit == 0 {
		gasLimit, err := m.GasEstimateGasLimit(ctx, msg, ts)
		if err != nil {
			return nil, err
		}
		msg.GasLimit = int64(float64(gasLimit) * m.Mpool.GetConfig().GasLimitOverestimation)

		// Gas overestimation can cause us to exceed the block gas limit, cap it.
		if msg.GasLimit > buildconstants.BlockGasLimit {
			msg.GasLimit = buildconstants.BlockGasLimit
		}
	}

	if msg.GasPremium == types.EmptyInt || types.BigCmp(msg.GasPremium, types.NewInt(0)) == 0 {
		gasPremium, err := m.GasEstimateGasPremium(ctx, 10, msg.From, msg.GasLimit, ts)
		if err != nil {
			return nil, xerrors.Errorf("estimating gas price: %w", err)
		}
		msg.GasPremium = gasPremium
	}

	if msg.GasFeeCap == types.EmptyInt || types.BigCmp(msg.GasFeeCap, types.NewInt(0)) == 0 {
		feeCap, err := m.GasEstimateFeeCap(ctx, msg, 20, ts)
		if err != nil {
			return nil, xerrors.Errorf("estimating fee cap: %w", err)
		}
		msg.GasFeeCap = feeCap
	}

	messagepool.CapGasFee(m.GetMaxFee, msg, spec)

	return msg, nil
}
