package stmgr

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/ipfs/go-cid"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

func (sm *StateManager) CallRaw(ctx context.Context, msg *types.Message, bstate cid.Cid, r vm.Rand, bheight abi.ChainEpoch) (*api.InvocResult, error) {
	ctx, span := trace.StartSpan(ctx, "statemanager.CallRaw")
	defer span.End()

	vmopt := &vm.VMOpts{
		StateBase:      bstate,
		Epoch:          bheight,
		Rand:           r,
		Bstore:         sm.cs.Blockstore(),
		Syscalls:       sm.cs.VMSys(),
		CircSupplyCalc: sm.GetCirculatingSupply,
		NtwkVersion:    sm.GetNtwkVersion,
		BaseFee:        types.NewInt(0),
	}

	vmi, err := vm.NewVM(vmopt)
	if err != nil {
		return nil, xerrors.Errorf("failed to set up vm: %w", err)
	}

	if msg.GasLimit == 0 {
		msg.GasLimit = build.BlockGasLimit
	}
	if msg.GasFeeCap == types.EmptyInt {
		msg.GasFeeCap = types.NewInt(0)
	}
	if msg.GasPremium == types.EmptyInt {
		msg.GasPremium = types.NewInt(0)
	}

	if msg.Value == types.EmptyInt {
		msg.Value = types.NewInt(0)
	}

	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.Int64Attribute("gas_limit", msg.GasLimit),
			trace.StringAttribute("gas_feecap", msg.GasFeeCap.String()),
			trace.StringAttribute("value", msg.Value.String()),
		)
	}

	fromActor, err := vmi.StateTree().GetActor(msg.From)
	if err != nil {
		return nil, xerrors.Errorf("call raw get actor: %s", err)
	}

	msg.Nonce = fromActor.Nonce

	// TODO: maybe just use the invoker directly?
	ret, err := vmi.ApplyImplicitMessage(ctx, msg)
	if err != nil {
		return nil, xerrors.Errorf("apply message failed: %w", err)
	}

	var errs string
	if ret.ActorErr != nil {
		errs = ret.ActorErr.Error()
		log.Warnf("chain call failed: %s", ret.ActorErr)
	}

	return &api.InvocResult{
		Msg:            msg,
		MsgRct:         &ret.MessageReceipt,
		ExecutionTrace: ret.ExecutionTrace,
		Error:          errs,
		Duration:       ret.Duration,
	}, nil

}

func (sm *StateManager) Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	state := ts.ParentState()

	r := store.NewChainRand(sm.cs, ts.Cids())

	return sm.CallRaw(ctx, msg, state, r, ts.Height())
}

func (sm *StateManager) CallWithGas(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet) (*api.InvocResult, error) {
	ctx, span := trace.StartSpan(ctx, "statemanager.CallWithGas")
	defer span.End()

	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	state, _, err := sm.TipSetState(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("computing tipset state: %w", err)
	}

	r := store.NewChainRand(sm.cs, ts.Cids())

	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.Int64Attribute("gas_limit", msg.GasLimit),
			trace.StringAttribute("gas_feecap", msg.GasFeeCap.String()),
			trace.StringAttribute("value", msg.Value.String()),
		)
	}

	vmopt := &vm.VMOpts{
		StateBase:      state,
		Epoch:          ts.Height() + 1,
		Rand:           r,
		Bstore:         sm.cs.Blockstore(),
		Syscalls:       sm.cs.VMSys(),
		CircSupplyCalc: sm.GetCirculatingSupply,
		NtwkVersion:    sm.GetNtwkVersion,
		BaseFee:        ts.Blocks()[0].ParentBaseFee,
	}
	vmi, err := vm.NewVM(vmopt)
	if err != nil {
		return nil, xerrors.Errorf("failed to set up vm: %w", err)
	}
	for i, m := range priorMsgs {
		_, err := vmi.ApplyMessage(ctx, m)
		if err != nil {
			return nil, xerrors.Errorf("applying prior message (%d, %s): %w", i, m.Cid(), err)
		}
	}

	fromActor, err := vmi.StateTree().GetActor(msg.From)
	if err != nil {
		return nil, xerrors.Errorf("call raw get actor: %s", err)
	}

	msg.Nonce = fromActor.Nonce

	fromKey, err := sm.ResolveToKeyAddress(ctx, msg.From, ts)
	if err != nil {
		return nil, xerrors.Errorf("could not resolve key: %w", err)
	}

	var msgApply types.ChainMsg

	switch fromKey.Protocol() {
	case address.BLS:
		msgApply = msg
	case address.SECP256K1:
		msgApply = &types.SignedMessage{
			Message: *msg,
			Signature: crypto.Signature{
				Type: crypto.SigTypeSecp256k1,
				Data: make([]byte, 65),
			},
		}

	}

	ret, err := vmi.ApplyMessage(ctx, msgApply)
	if err != nil {
		return nil, xerrors.Errorf("apply message failed: %w", err)
	}

	var errs string
	if ret.ActorErr != nil {
		errs = ret.ActorErr.Error()
	}

	return &api.InvocResult{
		Msg:            msg,
		MsgRct:         &ret.MessageReceipt,
		ExecutionTrace: ret.ExecutionTrace,
		Error:          errs,
		Duration:       ret.Duration,
	}, nil
}

var errHaltExecution = fmt.Errorf("halt")

func (sm *StateManager) Replay(ctx context.Context, ts *types.TipSet, mcid cid.Cid) (*types.Message, *vm.ApplyRet, error) {
	var outm *types.Message
	var outr *vm.ApplyRet

	_, _, err := sm.computeTipSetState(ctx, ts, func(c cid.Cid, m *types.Message, ret *vm.ApplyRet) error {
		if c == mcid {
			outm = m
			outr = ret
			return errHaltExecution
		}
		return nil
	})
	if err != nil && err != errHaltExecution {
		return nil, nil, xerrors.Errorf("unexpected error during execution: %w", err)
	}

	if outr == nil {
		return nil, nil, xerrors.Errorf("given message not found in tipset")
	}

	return outm, outr, nil
}
