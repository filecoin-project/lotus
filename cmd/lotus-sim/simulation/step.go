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
	// The number of expected blocks in a tipset. We use this to determine how much gas a tipset
	// has.
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

// step steps the simulation state forward one step, producing and executing a new tipset.
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

// popNextMessages generates/picks a set of messages to be included in the next block.
//
// - This function is destructive and should only be called once per epoch.
// - This function does not store anything in the repo.
// - This function handles all gas estimation. The returned messages should all fit in a single
//   block.
func (ss *simulationState) popNextMessages(ctx context.Context) ([]*types.Message, error) {
	parentTs := ss.head

	// First we make sure we don't have an upgrade at this epoch. If we do, we return no
	// messages so we can just create an empty block at that epoch.
	//
	// This isn't what the network does, but it makes things easier. Otherwise, we'd need to run
	// migrations before this epoch and I'd rather not deal with that.
	nextHeight := parentTs.Height() + 1
	prevVer := ss.sm.GetNtwkVersion(ctx, nextHeight-1)
	nextVer := ss.sm.GetNtwkVersion(ctx, nextHeight)
	if nextVer != prevVer {
		log.Warnw("packing no messages for version upgrade block",
			"old", prevVer,
			"new", nextVer,
			"epoch", nextHeight,
		)
		return nil, nil
	}

	// Next, we compute the state for the parent tipset. In practice, this will likely be
	// cached.
	parentState, _, err := ss.sm.TipSetState(ctx, parentTs)
	if err != nil {
		return nil, err
	}

	// Then we construct a VM to execute messages for gas estimation.
	//
	// Most parts of this VM are "real" except:
	// 1. We don't charge a fee.
	// 2. The runtime has "fake" proof logic.
	// 3. We don't actually save any of the results.
	r := store.NewChainRand(ss.sm.ChainStore(), parentTs.Cids())
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

	// Next we define a helper function for "pushing" messages. This is the function that will
	// be passed to the "pack" functions.
	//
	// It.
	//
	// 1. Tries to execute the message on-top-of the already pushed message.
	// 2. Is careful to revert messages on failure to avoid nasties like nonce-gaps.
	// 3. Resolves IDs as necessary, fills in missing parts of the message, etc.
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

	// Finally, we generate a set of messages to be included in
	if err := ss.packMessages(ctx, tryPushMsg); err != nil {
		return nil, err
	}

	return messages, nil
}

// functionName extracts the name of given function.
func functionName(fn interface{}) string {
	name := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	lastDot := strings.LastIndexByte(name, '.')
	if lastDot >= 0 {
		name = name[lastDot+1 : len(name)-3]
	}
	lastDash := strings.LastIndexByte(name, '-')
	if lastDash > 0 {
		name = name[:lastDash]
	}
	return name
}

// packMessages packs messages with the given packFunc until the block is full (packFunc returns
// true).
// TODO: Make this more configurable for other simulations.
func (ss *simulationState) packMessages(ctx context.Context, cb packFunc) error {
	type messageGenerator func(ctx context.Context, cb packFunc) (full bool, err error)

	// We pack messages in-order:
	// 1. Any window posts. We pack window posts as soon as the deadline opens to ensure we only
	//    miss them if/when we run out of chain bandwidth.
	// 2. Prove commits. We do this eagerly to ensure they don't expire.
	// 3. Finally, we fill the rest of the space with pre-commits.
	messageGenerators := []messageGenerator{
		ss.packWindowPoSts,
		ss.packProveCommits,
		ss.packPreCommits,
	}

	for _, mgen := range messageGenerators {
		// We're intentionally ignoring the "full" signal so we can try to pack a few more
		// messages.
		_, err := mgen(ctx, cb)
		if err != nil {
			return xerrors.Errorf("when packing messages with %s: %w", functionName(mgen), err)
		}
	}
	return nil
}
