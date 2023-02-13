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
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"

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
func (sm *StateManager) Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	// Copy the message as we modify it below.
	msgCopy := *msg
	msg = &msgCopy

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

	return sm.callInternal(ctx, msg, nil, ts, cid.Undef, sm.GetNetworkVersion, false, false)
}

// CallWithGas calculates the state for a given tipset, and then applies the given message on top of that state.
func (sm *StateManager) CallWithGas(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet) (*api.InvocResult, error) {
	return sm.callInternal(ctx, msg, priorMsgs, ts, cid.Undef, sm.GetNetworkVersion, true, true)
}

// CallAtStateAndVersion allows you to specify a message to execute on the given stateCid and network version.
// This should mostly be used for gas modelling on a migrated state.
// Tipset here is not needed because stateCid and network version fully describe execution we want. The internal function
// will get the heaviest tipset for use for things like basefee, which we don't really care about here.
func (sm *StateManager) CallAtStateAndVersion(ctx context.Context, msg *types.Message, stateCid cid.Cid, v network.Version) (*api.InvocResult, error) {
	nvGetter := func(context.Context, abi.ChainEpoch) network.Version {
		return v
	}

	return sm.callInternal(ctx, msg, nil, nil, stateCid, nvGetter, true, false)
}

//   - If no tipset is specified, the first tipset without an expensive migration or one in its parent is used.
//   - If executing a message at a given tipset or its parent would trigger an expensive migration, the call will
//     fail with ErrExpensiveFork.
func (sm *StateManager) callInternal(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet, stateCid cid.Cid, nvGetter rand.NetworkVersionGetter, checkGas, applyTsMessages bool) (*api.InvocResult, error) {
	ctx, span := trace.StartSpan(ctx, "statemanager.callInternal")
	defer span.End()

	// Copy the message as we'll be modifying the nonce.
	msgCopy := *msg
	msg = &msgCopy

	var err error
	var pts *types.TipSet
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()

		// Search back till we find a height with no fork, or we reach the beginning.
		// We need the _previous_ height to have no fork, because we'll
		// run the fork logic in `sm.TipSetState`. We need the _current_
		// height to have no fork, because we'll run it inside this
		// function before executing the given message.
		for ts.Height() > 0 {
			pts, err = sm.cs.GetTipSetFromKey(ctx, ts.Parents())
			if err != nil {
				return nil, xerrors.Errorf("failed to find a non-forking epoch: %w", err)
			}
			// Checks for expensive forks from the parents to the tipset, including nil tipsets
			if !sm.hasExpensiveForkBetween(pts.Height(), ts.Height()+1) {
				break
			}

			ts = pts
		}
	} else if ts.Height() > 0 {
		pts, err = sm.cs.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("failed to find a non-forking epoch: %w", err)
		}
		if sm.hasExpensiveForkBetween(pts.Height(), ts.Height()+1) {
			return nil, ErrExpensiveFork
		}
	}

	// Unless executing on a specific state cid, apply all the messages from the current tipset
	// first. Unfortunately, we can't just execute the tipset, because that will run cron. We
	// don't want to apply miner messages after cron runs in a given epoch.
	if stateCid == cid.Undef {
		stateCid = ts.ParentState()
	}
	if applyTsMessages {
		tsMsgs, err := sm.cs.MessagesForTipset(ctx, ts)
		if err != nil {
			return nil, xerrors.Errorf("failed to lookup messages for parent tipset: %w", err)
		}
		priorMsgs = append(tsMsgs, priorMsgs...)
	}

	// Technically, the tipset we're passing in here should be ts+1, but that may not exist.
	stateCid, err = sm.HandleStateForks(ctx, stateCid, ts.Height(), nil, ts)
	if err != nil {
		return nil, fmt.Errorf("failed to handle fork: %w", err)
	}

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
		Epoch:          ts.Height(),
		Timestamp:      ts.MinTimestamp(),
		Rand:           rand.NewStateRand(sm.cs, ts.Cids(), sm.beacon, nvGetter),
		Bstore:         buffStore,
		Actors:         sm.tsExec.NewActorRegistry(),
		Syscalls:       sm.Syscalls,
		CircSupplyCalc: sm.GetVMCirculatingSupply,
		NetworkVersion: nvGetter(ctx, ts.Height()),
		BaseFee:        ts.Blocks()[0].ParentBaseFee,
		LookbackState:  LookbackStateGetterForTipset(sm, ts),
		TipSetGetter:   TipSetGetterForTipset(sm.cs, ts),
		Tracing:        true,
	}
	vmi, err := sm.newVM(ctx, vmopt)
	if err != nil {
		return nil, xerrors.Errorf("failed to set up vm: %w", err)
	}
	for i, m := range priorMsgs {
		_, err = vmi.ApplyMessage(ctx, m)
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

	// If the fee cap is set to zero, make gas free.
	if msg.GasFeeCap.NilOrZero() {
		// Now estimate with a new VM with no base fee.
		vmopt.BaseFee = big.Zero()
		vmopt.StateBase = stateCid

		vmi, err = sm.newVM(ctx, vmopt)
		if err != nil {
			return nil, xerrors.Errorf("failed to set up estimation vm: %w", err)
		}
	}

	var ret *vm.ApplyRet
	var gasInfo api.MsgGasCost
	if checkGas {
		fromKey, err := sm.ResolveToDeterministicAddress(ctx, msg.From, ts)
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
		case address.Delegated:
			msgApply = &types.SignedMessage{
				Message: *msg,
				Signature: crypto.Signature{
					Type: crypto.SigTypeDelegated,
					Data: make([]byte, 65),
				},
			}
		}

		ret, err = vmi.ApplyMessage(ctx, msgApply)
		if err != nil {
			return nil, xerrors.Errorf("gas estimation failed: %w", err)
		}
		gasInfo = MakeMsgGasCost(msg, ret)
	} else {
		ret, err = vmi.ApplyImplicitMessage(ctx, msg)
		if err != nil && ret == nil {
			return nil, xerrors.Errorf("apply message failed: %w", err)
		}
	}

	var errs string
	if ret.ActorErr != nil {
		errs = ret.ActorErr.Error()
	}

	return &api.InvocResult{
		MsgCid:         msg.Cid(),
		Msg:            msg,
		MsgRct:         &ret.MessageReceipt,
		GasCost:        gasInfo,
		ExecutionTrace: ret.ExecutionTrace,
		Error:          errs,
		Duration:       ret.Duration,
	}, err
}

var errHaltExecution = fmt.Errorf("halt")

func (sm *StateManager) Replay(ctx context.Context, ts *types.TipSet, mcid cid.Cid) (*types.Message, *vm.ApplyRet, error) {
	var finder messageFinder
	// message to find
	finder.mcid = mcid

	_, _, err := sm.tsExec.ExecuteTipSet(ctx, sm, ts, &finder, true)
	if err != nil && !xerrors.Is(err, errHaltExecution) {
		return nil, nil, xerrors.Errorf("unexpected error during execution: %w", err)
	}

	if finder.outr == nil {
		return nil, nil, xerrors.Errorf("given message not found in tipset")
	}

	return finder.outm, finder.outr, nil
}
