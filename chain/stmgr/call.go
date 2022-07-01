package stmgr

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

var ErrExpensiveFork = errors.New("refusing explicit call due to state fork at epoch")

// Call applies the given message to the given tipset's parent state, at the epoch following the
// tipset's parent. In the presence of null blocks, the height at which the message is invoked may
// be less than the specified tipset.
//
// - If no tipset is specified, the first tipset without an expensive migration is used.
// - If executing a message at a given tipset would trigger an expensive migration, the call will
//   fail with ErrExpensiveFork.
func (sm *StateManager) Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	ctx, span := trace.StartSpan(ctx, "statemanager.Call")
	defer span.End()

	var pheight abi.ChainEpoch = -1

	// If no tipset is provided, try to find one without a fork.
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
		// Search back till we find a height with no fork, or we reach the beginning.
		for ts.Height() > 0 {
			pts, err := sm.cs.GetTipSetFromKey(ctx, ts.Parents())
			if err != nil {
				return nil, xerrors.Errorf("failed to find a non-forking epoch: %w", err)
			}
			if !sm.hasExpensiveFork(pts.Height()) {
				pheight = pts.Height()
				break
			}
			ts = pts
		}
	} else if ts.Height() > 0 {
		pts, err := sm.cs.LoadTipSet(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("failed to load parent tipset: %w", err)
		}
		pheight = pts.Height()
		if sm.hasExpensiveFork(pheight) {
			return nil, ErrExpensiveFork
		}
	} else {
		// We can't get the parent tipset in this case.
		pheight = ts.Height() - 1
	}

	// Since we're simulating a future message, pretend we're applying it in the "next" tipset
	vmHeight := pheight + 1
	bstate := ts.ParentState()

	// Run the (not expensive) migration.
	bstate, err := sm.HandleStateForks(ctx, bstate, pheight, nil, ts)
	if err != nil {
		return nil, fmt.Errorf("failed to handle fork: %w", err)
	}

	vmopt := &vm.VMOpts{
		StateBase:      bstate,
		Epoch:          vmHeight,
		Rand:           rand.NewStateRand(sm.cs, ts.Cids(), sm.beacon, sm.GetNetworkVersion),
		Bstore:         sm.cs.StateBlockstore(),
		Actors:         sm.tsExec.NewActorRegistry(),
		Syscalls:       sm.Syscalls,
		CircSupplyCalc: sm.GetVMCirculatingSupply,
		NetworkVersion: sm.GetNetworkVersion(ctx, pheight+1),
		BaseFee:        types.NewInt(0),
		LookbackState:  LookbackStateGetterForTipset(sm, ts),
	}

	vmi, err := sm.newVM(ctx, vmopt)
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

	stTree, err := sm.StateTree(bstate)
	if err != nil {
		return nil, xerrors.Errorf("failed to load state tree: %w", err)
	}

	fromActor, err := stTree.GetActor(msg.From)
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
		MsgCid:         msg.Cid(),
		Msg:            msg,
		MsgRct:         &ret.MessageReceipt,
		ExecutionTrace: ret.ExecutionTrace,
		Error:          errs,
		Duration:       ret.Duration,
	}, nil

}

func (sm *StateManager) CallWithGas(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet) (*api.InvocResult, error) {
	ctx, span := trace.StartSpan(ctx, "statemanager.CallWithGas")
	defer span.End()

	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()

		// Search back till we find a height with no fork, or we reach the beginning.
		// We need the _previous_ height to have no fork, because we'll
		// run the fork logic in `sm.TipSetState`. We need the _current_
		// height to have no fork, because we'll run it inside this
		// function before executing the given message.
		for ts.Height() > 0 {
			pts, err := sm.cs.GetTipSetFromKey(ctx, ts.Parents())
			if err != nil {
				return nil, xerrors.Errorf("failed to find a non-forking epoch: %w", err)
			}
			if !sm.hasExpensiveForkBetween(pts.Height(), ts.Height()+1) {
				break
			}

			ts = pts
		}
	} else if ts.Height() > 0 {
		pts, err := sm.cs.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("failed to find a non-forking epoch: %w", err)
		}
		if sm.hasExpensiveForkBetween(pts.Height(), ts.Height()+1) {
			return nil, ErrExpensiveFork
		}
	}

	// Since we're simulating a future message, pretend we're applying it in the "next" tipset
	vmHeight := ts.Height() + 1

	stateCid, _, err := sm.TipSetState(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("computing tipset state: %w", err)
	}

	// Technically, the tipset we're passing in here should be ts+1, but that may not exist.
	stateCid, err = sm.HandleStateForks(ctx, stateCid, ts.Height(), nil, ts)
	if err != nil {
		return nil, fmt.Errorf("failed to handle fork: %w", err)
	}

	r := rand.NewStateRand(sm.cs, ts.Cids(), sm.beacon, sm.GetNetworkVersion)

	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.Int64Attribute("gas_limit", msg.GasLimit),
			trace.StringAttribute("gas_feecap", msg.GasFeeCap.String()),
			trace.StringAttribute("value", msg.Value.String()),
		)
	}

	buffStore := blockstore.NewTieredBstore(sm.cs.StateBlockstore(), blockstore.NewMemorySync())
	vmopt := &vm.VMOpts{
		StateBase:      stateCid,
		Epoch:          vmHeight,
		Rand:           r,
		Bstore:         buffStore,
		Actors:         sm.tsExec.NewActorRegistry(),
		Syscalls:       sm.Syscalls,
		CircSupplyCalc: sm.GetVMCirculatingSupply,
		NetworkVersion: sm.GetNetworkVersion(ctx, ts.Height()+1),
		BaseFee:        ts.Blocks()[0].ParentBaseFee,
		LookbackState:  LookbackStateGetterForTipset(sm, ts),
	}
	vmi, err := sm.newVM(ctx, vmopt)
	if err != nil {
		return nil, xerrors.Errorf("failed to set up vm: %w", err)
	}
	for i, m := range priorMsgs {
		_, err := vmi.ApplyMessage(ctx, m)
		if err != nil {
			return nil, xerrors.Errorf("applying prior message (%d, %s): %w", i, m.Cid(), err)
		}
	}

	// We flush to get the VM's view of the state tree after applying the above messages
	// This is needed to get the correct nonce from the actor state to match the VM
	stateCid, err = vmi.Flush(ctx)
	if err != nil {
		return nil, xerrors.Errorf("flushing vm: %w", err)
	}

	stTree, err := state.LoadStateTree(cbor.NewCborStore(buffStore), stateCid)
	if err != nil {
		return nil, xerrors.Errorf("loading state tree: %w", err)
	}

	fromActor, err := stTree.GetActor(msg.From)
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
		MsgCid:         msg.Cid(),
		Msg:            msg,
		MsgRct:         &ret.MessageReceipt,
		GasCost:        MakeMsgGasCost(msg, ret),
		ExecutionTrace: ret.ExecutionTrace,
		Error:          errs,
		Duration:       ret.Duration,
	}, nil
}

var errHaltExecution = fmt.Errorf("halt")

func (sm *StateManager) Replay(ctx context.Context, ts *types.TipSet, mcid cid.Cid) (*types.Message, *vm.ApplyRet, error) {
	var finder messageFinder
	// message to find
	finder.mcid = mcid

	_, _, err := sm.tsExec.ExecuteTipSet(ctx, sm, ts, &finder)
	if err != nil && !xerrors.Is(err, errHaltExecution) {
		return nil, nil, xerrors.Errorf("unexpected error during execution: %w", err)
	}

	if finder.outr == nil {
		return nil, nil, xerrors.Errorf("given message not found in tipset")
	}

	return finder.outm, finder.outr, nil
}
