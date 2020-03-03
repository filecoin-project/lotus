package stmgr

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

func (sm *StateManager) CallRaw(ctx context.Context, msg *types.Message, bstate cid.Cid, r vm.Rand, bheight uint64) (*api.InvocResult, error) {
	ctx, span := trace.StartSpan(ctx, "statemanager.CallRaw")
	defer span.End()

	vmi, err := vm.NewVM(bstate, bheight, r, actors.NetworkAddress, sm.cs.Blockstore(), sm.cs.VMSys())
	if err != nil {
		return nil, xerrors.Errorf("failed to set up vm: %w", err)
	}

	if msg.GasLimit == types.EmptyInt {
		msg.GasLimit = types.NewInt(10000000000)
	}
	if msg.GasPrice == types.EmptyInt {
		msg.GasPrice = types.NewInt(0)
	}
	if msg.Value == types.EmptyInt {
		msg.Value = types.NewInt(0)
	}

	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.Int64Attribute("gas_limit", int64(msg.GasLimit.Uint64())),
			trace.Int64Attribute("gas_price", int64(msg.GasPrice.Uint64())),
			trace.StringAttribute("value", msg.Value.String()),
		)
	}

	fromActor, err := vmi.StateTree().GetActor(msg.From)
	if err != nil {
		return nil, xerrors.Errorf("call raw get actor: %s", err)
	}

	msg.Nonce = fromActor.Nonce

	// TODO: maybe just use the invoker directly?
	ret, err := vmi.ApplyMessage(ctx, msg)
	if err != nil {
		return nil, xerrors.Errorf("apply message failed: %w", err)
	}

	var errs string
	if ret.ActorErr != nil {
		errs = ret.ActorErr.Error()
		log.Warnf("chain call failed: %s", ret.ActorErr)
	}
	return &api.InvocResult{
		Msg: 		msg,
		MsgRct: 	&ret.MessageReceipt,
		Error:  	errs,
	}, nil

}

func (sm *StateManager) Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	state := ts.ParentState()

	r := store.NewChainRand(sm.cs, ts.Cids(), ts.Height())

	return sm.CallRaw(ctx, msg, state, r, ts.Height())
}

var errHaltExecution = fmt.Errorf("halt")

func (sm *StateManager) Replay(ctx context.Context, ts *types.TipSet, mcid cid.Cid) (*types.Message, *vm.ApplyRet, error) {
	var outm *types.Message
	var outr *vm.ApplyRet

	_, _, err := sm.computeTipSetState(ctx, ts.Blocks(), func(c cid.Cid, m *types.Message, ret *vm.ApplyRet) error {
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

	return outm, outr, nil
}
