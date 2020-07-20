package full

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
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

func (a *GasAPI) GasEstimateGasLimit(ctx context.Context, msgIn *types.Message,
	tsk types.TipSetKey) (int64, error) {

	msg := *msgIn
	msg.GasLimit = build.BlockGasLimit
	msg.GasPrice = types.NewInt(1)

	ts, err := a.Cs.GetTipSetFromKey(tsk)
	if err != nil {
		return -1, xerrors.Errorf("could not get tipset: %w", err)
	}

	res, err := a.Stmgr.CallWithGas(ctx, &msg, ts)
	if err != nil {
		return -1, xerrors.Errorf("CallWithGas failed: %w", err)
	}
	if res.MsgRct.ExitCode != exitcode.Ok {
		return -1, xerrors.Errorf("message execution failed: exit %s, reason: %s", res.MsgRct.ExitCode, res.Error)
	}

	return res.MsgRct.GasUsed, nil
}
