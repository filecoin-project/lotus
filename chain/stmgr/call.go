package stmgr

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"

	cid "github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

func (sm *StateManager) CallRaw(ctx context.Context, msg *types.Message, bstate cid.Cid, r vm.Rand, bheight uint64) (*types.MessageReceipt, error) {
	vmi, err := vm.NewVM(bstate, bheight, r, actors.NetworkAddress, sm.cs)
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

	fromActor, err := vmi.StateTree().GetActor(msg.From)
	if err != nil {
		return nil, err
	}

	msg.Nonce = fromActor.Nonce

	// TODO: maybe just use the invoker directly?
	ret, err := vmi.ApplyMessage(ctx, msg)
	if err != nil {
		return nil, xerrors.Errorf("apply message failed: %w", err)
	}

	if ret.ActorErr != nil {
		log.Warnf("chain call failed: %s", ret.ActorErr)
	}
	return &ret.MessageReceipt, nil

}

func (sm *StateManager) Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	state, err := sm.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	r := vm.NewChainRand(sm.cs, ts.Cids(), ts.Height(), nil)

	return sm.CallRaw(ctx, msg, state, r, ts.Height())
}

var errHaltExecution = fmt.Errorf("halt")

func (sm *StateManager) Replay(ctx context.Context, ts *types.TipSet, mcid cid.Cid) (*types.Message, *vm.ApplyRet, error) {
	var outm *types.Message
	var outr *vm.ApplyRet

	_, err := sm.computeTipSetState(ctx, ts.Blocks(), func(c cid.Cid, m *types.Message, ret *vm.ApplyRet) error {
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
