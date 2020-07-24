package full

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

type GasAPI struct {
	fx.In
	Stmgr *stmgr.StateManager
	Cs    *store.ChainStore
	Mpool *messagepool.MessagePool
}

const MinGasPrice = 1

func (a *GasAPI) GasEstimateGasPrice(ctx context.Context, nblocksincl uint64,
	sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error) {

	// TODO: something smarter obviously
	switch nblocksincl {
	case 0:
		return types.NewInt(MinGasPrice + 2), nil
	case 1:
		return types.NewInt(MinGasPrice + 1), nil
	default:
		return types.NewInt(MinGasPrice), nil
	}
}

func (a *GasAPI) GasEstimateGasLimit(ctx context.Context, msgIn *types.Message, _ types.TipSetKey) (int64, error) {

	msg := *msgIn
	msg.GasLimit = build.BlockGasLimit
	msg.GasPrice = types.NewInt(1)

	currTs := a.Cs.GetHeaviestTipSet()
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
