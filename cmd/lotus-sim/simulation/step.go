package simulation

import (
	"context"
	"reflect"
	"runtime"
	"strings"

	"github.com/filecoin-project/go-address"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/account"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

const (
	expectedBlocks = 5
	// TODO: This will produce invalid blocks but it will accurately model the amount of gas
	// we're willing to use per-tipset.
	// A more correct approach would be to produce 5 blocks. We can do that later.
	targetGas = build.BlockGasTarget * expectedBlocks
)

var baseFee = abi.NewTokenAmount(0)

// Step steps the simulation forward one step. This may move forward by more than one epoch.
func (sim *Simulation) Step(ctx context.Context) (*types.TipSet, error) {
	state, err := sim.simState(ctx)
	if err != nil {
		return nil, err
	}
	ts, err := state.step(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to step simulation: %w", err)
	}
	return ts, nil
}

func (ss *simulationState) step(ctx context.Context) (*types.TipSet, error) {
	log.Infow("step", "epoch", ss.head.Height()+1)
	messages, err := ss.popNextMessages(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to select messages for block: %w", err)
	}
	head, err := ss.makeTipSet(ctx, messages)
	if err != nil {
		return nil, xerrors.Errorf("failed to make tipset: %w", err)
	}
	if err := ss.SetHead(head); err != nil {
		return nil, xerrors.Errorf("failed to update head: %w", err)
	}
	return head, nil
}

type packFunc func(*types.Message) (full bool, err error)
type messageGenerator func(ctx context.Context, cb packFunc) (full bool, err error)

func (ss *simulationState) popNextMessages(ctx context.Context) ([]*types.Message, error) {
	parentTs := ss.head
	parentState, _, err := ss.sm.TipSetState(ctx, parentTs)
	if err != nil {
		return nil, err
	}
	nextHeight := parentTs.Height() + 1
	prevVer := ss.sm.GetNtwkVersion(ctx, nextHeight-1)
	nextVer := ss.sm.GetNtwkVersion(ctx, nextHeight)
	if nextVer != prevVer {
		// So... we _could_ actually run the migration, but that's a pain. It's easier to
		// just have an empty block then let the state manager run the migration as normal.
		log.Warnw("packing no messages for version upgrade block",
			"old", prevVer,
			"new", nextVer,
			"epoch", nextHeight,
		)
		return nil, nil
	}

	// Then we need to execute messages till we run out of gas. Those messages will become the
	// block's messages.
	r := store.NewChainRand(ss.sm.ChainStore(), parentTs.Cids())
	// TODO: Factor this out maybe?
	vmopt := &vm.VMOpts{
		StateBase:      parentState,
		Epoch:          nextHeight,
		Rand:           r,
		Bstore:         ss.sm.ChainStore().StateBlockstore(),
		Syscalls:       ss.sm.ChainStore().VMSys(),
		CircSupplyCalc: ss.sm.GetVMCirculatingSupply,
		NtwkVersion:    ss.sm.GetNtwkVersion,
		BaseFee:        abi.NewTokenAmount(0), // FREE!
		LookbackState:  stmgr.LookbackStateGetterForTipset(ss.sm, parentTs),
	}
	vmi, err := vm.NewVM(ctx, vmopt)
	if err != nil {
		return nil, err
	}
	// TODO: This is the wrong store and may not include important state for what we're doing
	// here....
	// Maybe we just track nonces separately? Yeah, probably better that way.
	vmStore := vmi.ActorStore(ctx)
	var gasTotal int64
	var messages []*types.Message
	tryPushMsg := func(msg *types.Message) (bool, error) {
		if gasTotal >= targetGas {
			return true, nil
		}

		// Copy the message before we start mutating it.
		msgCpy := *msg
		msg = &msgCpy
		st := vmi.StateTree().(*state.StateTree)

		actor, err := st.GetActor(msg.From)
		if err != nil {
			return false, err
		}
		msg.Nonce = actor.Nonce
		if msg.From.Protocol() == address.ID {
			state, err := account.Load(vmStore, actor)
			if err != nil {
				return false, err
			}
			msg.From, err = state.PubkeyAddress()
			if err != nil {
				return false, err
			}
		}

		// TODO: Our gas estimation is broken for payment channels due to horrible hacks in
		// gasEstimateGasLimit.
		if msg.Value == types.EmptyInt {
			msg.Value = abi.NewTokenAmount(0)
		}
		msg.GasPremium = abi.NewTokenAmount(0)
		msg.GasFeeCap = abi.NewTokenAmount(0)
		msg.GasLimit = build.BlockGasLimit

		// We manually snapshot so we can revert nonce changes, etc. on failure.
		st.Snapshot(ctx)
		defer st.ClearSnapshot()

		ret, err := vmi.ApplyMessage(ctx, msg)
		if err != nil {
			_ = st.Revert()
			return false, err
		}
		if ret.ActorErr != nil {
			_ = st.Revert()
			return false, ret.ActorErr
		}

		// Sometimes there are bugs. Let's catch them.
		if ret.GasUsed == 0 {
			_ = st.Revert()
			return false, xerrors.Errorf("used no gas",
				"msg", msg,
				"ret", ret,
			)
		}

		// TODO: consider applying overestimation? We're likely going to "over pack" here by
		// ~25% because we're too accurate.

		// Did we go over? Yes, revert.
		newTotal := gasTotal + ret.GasUsed
		if newTotal > targetGas {
			_ = st.Revert()
			return true, nil
		}
		gasTotal = newTotal

		// Update the gas limit.
		msg.GasLimit = ret.GasUsed

		messages = append(messages, msg)
		return false, nil
	}
	for _, mgen := range []messageGenerator{ss.packWindowPoSts, ss.packProveCommits, ss.packPreCommits} {
		if full, err := mgen(ctx, tryPushMsg); err != nil {
			name := runtime.FuncForPC(reflect.ValueOf(mgen).Pointer()).Name()
			lastDot := strings.LastIndexByte(name, '.')
			fName := name[lastDot+1 : len(name)-3]
			return nil, xerrors.Errorf("when packing messages with %s: %w", fName, err)
		} else if full {
			break
		}
	}

	return messages, nil
}
