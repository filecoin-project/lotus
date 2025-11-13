package blockbuilder

import (
	"context"
	"math"

	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/account"
	"github.com/filecoin-project/lotus/chain/consensus"
	lrand "github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

const (
	// 0.25 is the default, but the number below is from the network.
	gasOverestimation = 1.0 / 0.808
	// The number of expected blocks in a tipset. We use this to determine how much gas a tipset
	// has.
	// 5 per tipset, but we effectively get 4 blocks worth of messages.
	expectedBlocks = 4
)

// TODO: This will produce invalid blocks but it will accurately model the amount of gas
// we're willing to use per-tipset.
// A more correct approach would be to produce 5 blocks. We can do that later.
var targetGas = buildconstants.BlockGasTarget * expectedBlocks

type BlockBuilder struct {
	ctx    context.Context
	logger *zap.SugaredLogger

	parentTs *types.TipSet
	parentSt *state.StateTree
	vm       *vm.LegacyVM
	sm       *stmgr.StateManager

	gasTotal int64
	messages []*types.Message
}

// NewBlockBuilder constructs a new block builder from the parent state. Use this to pack a block
// with messages.
//
// NOTE: The context applies to the life of the block builder itself (but does not need to be canceled).
func NewBlockBuilder(ctx context.Context, logger *zap.SugaredLogger, sm *stmgr.StateManager, parentTs *types.TipSet) (*BlockBuilder, error) {
	parentState, _, err := sm.TipSetState(ctx, parentTs)
	if err != nil {
		return nil, err
	}
	parentSt, err := sm.StateTree(parentState)
	if err != nil {
		return nil, err
	}

	bb := &BlockBuilder{
		ctx:      ctx,
		logger:   logger.With("epoch", parentTs.Height()+1),
		sm:       sm,
		parentTs: parentTs,
		parentSt: parentSt,
	}

	// Then we construct a LegacyVM to execute messages for gas estimation.
	//
	// Most parts of this LegacyVM are "real" except:
	// 1. We don't charge a fee.
	// 2. The runtime has "fake" proof logic.
	// 3. We don't actually save any of the results.
	r := lrand.NewStateRand(sm.ChainStore(), parentTs.Cids(), sm.Beacon(), sm.GetNetworkVersion)
	vmopt := &vm.VMOpts{
		StateBase:      parentState,
		Epoch:          parentTs.Height() + 1,
		Rand:           r,
		Bstore:         sm.ChainStore().StateBlockstore(),
		Actors:         consensus.NewActorRegistry(),
		Syscalls:       sm.VMSys(),
		CircSupplyCalc: sm.GetVMCirculatingSupply,
		NetworkVersion: sm.GetNetworkVersion(ctx, parentTs.Height()+1),
		BaseFee:        abi.NewTokenAmount(0),
		LookbackState:  stmgr.LookbackStateGetterForTipset(sm, parentTs),
	}
	bb.vm, err = vm.NewLegacyVM(bb.ctx, vmopt)
	if err != nil {
		return nil, err
	}
	return bb, nil
}

// PushMessage tries to push the specified message into the block.
//
// 1. All messages will be executed in-order.
// 2. Gas computation & nonce selection will be handled internally.
// 3. The base-fee is 0 so the sender does not need funds.
// 4. As usual, the sender must be an account (any account).
// 5. If the message fails to execute, this method will fail.
//
// Returns ErrOutOfGas when out of gas. Check BlockBuilder.GasRemaining and try pushing a cheaper
// message.
func (bb *BlockBuilder) PushMessage(msg *types.Message) (*types.MessageReceipt, error) {
	if bb.gasTotal >= targetGas {
		return nil, new(ErrOutOfGas)
	}

	st := bb.StateTree()
	store := bb.ActorStore()

	// Copy the message before we start mutating it.
	msgCpy := *msg
	msg = &msgCpy

	actor, err := st.GetActor(msg.From)
	if err != nil {
		return nil, err
	}
	if !builtin.IsAccountActor(actor.Code) {
		return nil, xerrors.Errorf(
			"messages may only be sent from account actors, got message from %s (%s)",
			msg.From, builtin.ActorNameByCode(actor.Code),
		)
	}
	msg.Nonce = actor.Nonce
	if msg.From.Protocol() == address.ID {
		state, err := account.Load(store, actor)
		if err != nil {
			return nil, err
		}
		msg.From, err = state.PubkeyAddress()
		if err != nil {
			return nil, err
		}
	}

	// TODO: Our gas estimation is broken for payment channels due to horrible hacks in
	// gasEstimateGasLimit.
	if msg.Value == types.EmptyInt {
		msg.Value = abi.NewTokenAmount(0)
	}
	msg.GasPremium = abi.NewTokenAmount(0)
	msg.GasFeeCap = abi.NewTokenAmount(0)
	msg.GasLimit = buildconstants.BlockGasTarget

	// We manually snapshot so we can revert nonce changes, etc. on failure.
	err = st.Snapshot(bb.ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to take a snapshot while estimating message gas: %w", err)
	}
	defer st.ClearSnapshot()

	ret, err := bb.vm.ApplyMessage(bb.ctx, msg)
	if err != nil {
		_ = st.Revert()
		return nil, err
	}
	if ret.ActorErr != nil {
		_ = st.Revert()
		return nil, ret.ActorErr
	}

	// Sometimes there are bugs. Let's catch them.
	if ret.GasUsed == 0 {
		_ = st.Revert()
		return nil, xerrors.Errorf("used no gas %v -> %v", msg, ret)
	}

	// Update the gas limit taking overestimation into account.
	msg.GasLimit = int64(math.Ceil(float64(ret.GasUsed) * gasOverestimation))

	// Did we go over? Yes, revert.
	newTotal := bb.gasTotal + msg.GasLimit
	if newTotal > targetGas {
		_ = st.Revert()
		return nil, &ErrOutOfGas{Available: targetGas - bb.gasTotal, Required: msg.GasLimit}
	}
	bb.gasTotal = newTotal

	bb.messages = append(bb.messages, msg)
	return &ret.MessageReceipt, nil
}

// ActorStore returns the LegacyVM's current (pending) blockstore.
func (bb *BlockBuilder) ActorStore() adt.Store {
	return bb.vm.ActorStore(bb.ctx)
}

// StateTree returns the LegacyVM's current (pending) state-tree. This includes any changes made by
// successfully pushed messages.
//
// You probably want ParentStateTree
func (bb *BlockBuilder) StateTree() *state.StateTree {
	return bb.vm.StateTree().(*state.StateTree)
}

// ParentStateTree returns the parent state-tree (not the paren't tipset's parent state-tree).
func (bb *BlockBuilder) ParentStateTree() *state.StateTree {
	return bb.parentSt
}

// StateTreeByHeight will return a state-tree up through and including the current in-progress
// epoch.
//
// NOTE: This will return the state after the given epoch, not the parent state for the epoch.
func (bb *BlockBuilder) StateTreeByHeight(epoch abi.ChainEpoch) (*state.StateTree, error) {
	now := bb.Height()
	if epoch > now {
		return nil, xerrors.Errorf(
			"cannot load state-tree from future: %d > %d", epoch, bb.Height(),
		)
	} else if epoch <= 0 {
		return nil, xerrors.Errorf(
			"cannot load state-tree: epoch %d <= 0", epoch,
		)
	}

	// Manually handle "now" and "previous".
	switch epoch {
	case now:
		return bb.StateTree(), nil
	case now - 1:
		return bb.ParentStateTree(), nil
	}

	// Get the tipset of the block _after_ the target epoch so we can use its parent state.
	targetTs, err := bb.sm.ChainStore().GetTipsetByHeight(bb.ctx, epoch+1, bb.parentTs, false)
	if err != nil {
		return nil, err
	}

	return bb.sm.StateTree(targetTs.ParentState())
}

// Messages returns all messages currently packed into the next block.
// 1. DO NOT modify the slice, copy it.
// 2. DO NOT retain the slice, copy it.
func (bb *BlockBuilder) Messages() []*types.Message {
	return bb.messages
}

// GasRemaining returns the amount of remaining gas in the next block.
func (bb *BlockBuilder) GasRemaining() int64 {
	return targetGas - bb.gasTotal
}

// ParentTipSet returns the parent tipset.
func (bb *BlockBuilder) ParentTipSet() *types.TipSet {
	return bb.parentTs
}

// Height returns the epoch for the target block.
func (bb *BlockBuilder) Height() abi.ChainEpoch {
	return bb.parentTs.Height() + 1
}

// NetworkVersion returns the network version for the target block.
func (bb *BlockBuilder) NetworkVersion() network.Version {
	return bb.sm.GetNetworkVersion(bb.ctx, bb.Height())
}

// StateManager returns the stmgr.StateManager.
func (bb *BlockBuilder) StateManager() *stmgr.StateManager {
	return bb.sm
}

// ActorsVersion returns the actors version for the target block.
func (bb *BlockBuilder) ActorsVersion() (actorstypes.Version, error) {
	return actorstypes.VersionForNetwork(bb.NetworkVersion())
}

func (bb *BlockBuilder) L() *zap.SugaredLogger {
	return bb.logger
}
