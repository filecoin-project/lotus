package vm

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/types"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"
	"time"
)


func HandleMultiMsg(vm *VM, ctx context.Context, cmsg *types.Message, span *trace.Span, start time.Time)  (*ApplyRet, error) {
	msg := cmsg.VMMessage()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			//TODO
			trace.StringAttribute("to", msg.To.String()),
			trace.Int64Attribute("method", int64(msg.Method)),
			trace.StringAttribute("value", msg.Value.String()),
		)
	}

	if err := checkMessage(msg); err != nil {
		return nil, err
	}

	var MultiParams = new(types.ClassicalParams)
	err := MultiParams.Deserialize(cmsg.Params)
	if err != nil {
		return nil, xerrors.Errorf("invalid params: %w", err)
	}

	var gasTotalUsed int64 = 0
	var ErrCode exitcode.ExitCode
	var runT = new(Runtime)
	var ActorError aerrors.ActorError
	st := vm.cstate
	if err := st.Snapshot(ctx); err != nil {
		return nil, xerrors.Errorf("snapshot failed: %w", err)
	}
	defer st.ClearSnapshot()
	gasHolder := &types.Actor{Balance: types.NewInt(0)}
	gascost := types.BigMul(types.NewInt(uint64(cmsg.GasLimit)), cmsg.GasFeeCap)
	fromActor, err := st.GetActor(cmsg.From)
	// this should never happen, but is currently still exercised by some tests
	minerPenaltyAmount := types.BigMul(vm.baseFee, abi.NewTokenAmount(cmsg.GasLimit))
	if err != nil {
		if xerrors.Is(err, types.ErrActorNotFound) {
			gasOutputs := ZeroGasOutputs()
			gasOutputs.MinerPenalty = minerPenaltyAmount
			return &ApplyRet{
				MessageReceipt: types.MessageReceipt{
					ExitCode: exitcode.SysErrSenderInvalid,
					GasUsed:  0,
				},
				ActorErr: aerrors.Newf(exitcode.SysErrSenderInvalid, "actor not found: %s", cmsg.From),
				GasCosts: &gasOutputs,
				Duration: time.Since(start),
			}, nil
		}
		return nil, xerrors.Errorf("failed to look up from actor: %w", err)
	}

	// this should never happen, but is currently still exercised by some tests
	if !builtin.IsAccountActor(fromActor.Code) {
		gasOutputs := ZeroGasOutputs()
		gasOutputs.MinerPenalty = minerPenaltyAmount
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrSenderInvalid,
				GasUsed:  0,
			},
			ActorErr: aerrors.Newf(exitcode.SysErrSenderInvalid, "send from not account actor: %s", fromActor.Code),
			GasCosts: &gasOutputs,
			Duration: time.Since(start),
		}, nil
	}

	if cmsg.Nonce != fromActor.Nonce {
		gasOutputs := ZeroGasOutputs()
		gasOutputs.MinerPenalty = minerPenaltyAmount
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrSenderStateInvalid,
				GasUsed:  0,
			},
			ActorErr: aerrors.Newf(exitcode.SysErrSenderStateInvalid,
				"actor nonce invalid: msg:%d != state:%d", cmsg.Nonce, fromActor.Nonce),

			GasCosts: &gasOutputs,
			Duration: time.Since(start),
		}, nil
	}
	if fromActor.Balance.LessThan(gascost) {
		gasOutputs := ZeroGasOutputs()
		gasOutputs.MinerPenalty = minerPenaltyAmount
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrSenderStateInvalid,
				GasUsed:  0,
			},
			ActorErr: aerrors.Newf(exitcode.SysErrSenderStateInvalid,
				"actor balance less than needed: %s < %s", types.FIL(fromActor.Balance), types.FIL(gascost)),
			GasCosts: &gasOutputs,
			Duration: time.Since(start),
		}, nil
	}

	//gasHolder := &types.Actor{Balance: types.NewInt(0)}
	if err := vm.transferToGasHolder(cmsg.From, gasHolder, gascost); err != nil {
		return nil, xerrors.Errorf("failed to withdraw gas funds: %w", err)
	}
	if len(MultiParams.Params) == 0{
		return nil, xerrors.Errorf("empty params in multimsg is not permitted")
	}
	for _, v := range MultiParams.Params {
		newMsg := &types.Message{
			Version: cmsg.Version,
			To: v.To,
			From:cmsg.From,
			Nonce: cmsg.Nonce,
			Value: v.Value,
			GasLimit: cmsg.GasLimit,
			GasFeeCap: cmsg.GasFeeCap,
			GasPremium: cmsg.GasPremium,
			Method: v.Method,
			Params: v.Params,
		}
		//Msg Check.
		pl := PricelistByEpoch(vm.blockHeight)

		msgGas := pl.OnChainMessage(newMsg.ChainLength())
		runT.allowInternal = true
		ret, actorErr, rt := vm.send(ctx, newMsg, runT, &msgGas, start)
		if aerrors.IsFatal(actorErr) {
			return nil, xerrors.Errorf("[from=%s,to=%s,n=%d,m=%d,h=%d] fatal error: %w", newMsg.From, newMsg.To, newMsg.Nonce, newMsg.Method, vm.blockHeight, actorErr)
		}

		if actorErr != nil {
			log.Warnw("Send actor error", "from", newMsg.From, "to", newMsg.To, "nonce", newMsg.Nonce, "method", newMsg.Method, "height", vm.blockHeight, "error", fmt.Sprintf("%+v", actorErr))
		}

		if actorErr != nil && len(ret) != 0 {
			// This should not happen, something is wonky
			return nil, xerrors.Errorf("message invocation errored, but had a return value anyway: %w", actorErr)
		}

		if rt == nil {
			return nil, xerrors.Errorf("send returned nil runtime, send error was: %s", actorErr)
		}

		if len(ret) != 0 {
			// safely override actorErr since it must be nil
			actorErr = rt.chargeGasSafe(rt.Pricelist().OnChainReturnValue(len(ret)))
			if actorErr != nil {
				ret = nil
			}
		}

		var errcode exitcode.ExitCode

		if errcode = aerrors.RetCode(actorErr); errcode != 0 {
			// revert all state changes since snapshot
			if err := st.Revert(); err != nil {
				return nil, xerrors.Errorf("revert state failed: %w", err)
			}
		}
		runT = rt
	}

	runT.finilizeGasTracing()
	gasTotalUsed = runT.gasUsed

	//execute end.
	gasOutputs := ComputeGasOutputs(gasTotalUsed, cmsg.GasLimit, vm.baseFee, cmsg.GasFeeCap, cmsg.GasPremium, true)
	if len(MultiParams.Params) > 1 {
		//minerTip sale.
		gasOutputs.SaleMinerTip()
	}

	if err := vm.transferFromGasHolder(builtin.BurntFundsActorAddr, gasHolder,
		gasOutputs.BaseFeeBurn); err != nil {
		return nil, xerrors.Errorf("failed to burn base fee: %w", err)
	}

	if err := vm.transferFromGasHolder(reward.Address, gasHolder, gasOutputs.MinerTip); err != nil {
		return nil, xerrors.Errorf("failed to give miner gas reward: %w", err)
	}

	if err := vm.transferFromGasHolder(builtin.BurntFundsActorAddr, gasHolder,
		gasOutputs.OverEstimationBurn); err != nil {
		return nil, xerrors.Errorf("failed to burn overestimation fee: %w", err)
	}

	//refund unused gas
	if err := vm.transferFromGasHolder(cmsg.From, gasHolder, gasOutputs.Refund); err != nil {
		return nil, xerrors.Errorf("failed to refund gas: %w", err)
	}

	if types.BigCmp(types.NewInt(0), gasHolder.Balance) != 0 {
		return nil, xerrors.Errorf("gas handling math is wrong")
	}

	if err := vm.incrementNonce(msg.From); err != nil {
		return nil, err
	}
	return &ApplyRet{
		MessageReceipt: types.MessageReceipt{
			ExitCode: ErrCode,
			Return:   nil, //TODO ret.
			GasUsed:  gasTotalUsed,
		},
		ActorErr:       ActorError,
		ExecutionTrace: runT.executionTrace,
		GasCosts:       &gasOutputs,
		Duration:       time.Since(start),
	}, nil
}