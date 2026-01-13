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
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

type execMessageStrategy int

const (
	execNoMessages         execMessageStrategy = iota // apply no prior or current tipset messages
	execAllMessages                                   // apply all prior and current tipset messages
	execSameSenderMessages                            // apply all prior messages and any current tipset messages from the same sender
)

var ErrExpensiveFork = errors.New("refusing explicit call due to state fork at epoch")

// Call applies the given message to the given tipset's parent state, at the epoch following the
// tipset's parent. In the presence of null blocks, the height at which the message is invoked may
// be less than the specified tipset.
func (sm *StateManager) Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	return sm.CallOnState(ctx, cid.Undef, msg, ts)
}

func (sm *StateManager) CallOnState(ctx context.Context, stateCid cid.Cid, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	// Copy the message as we modify it below.
	msgCopy := *msg
	msg = &msgCopy

	if msg.GasLimit == 0 {
		msg.GasLimit = buildconstants.BlockGasLimit
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

	return sm.callInternal(ctx, msg, nil, ts, stateCid, sm.GetNetworkVersion, false, execSameSenderMessages, false)
}

// ApplyOnStateWithGas applies the given message on top of the given state root with gas tracing enabled
func (sm *StateManager) ApplyOnStateWithGas(ctx context.Context, stateCid cid.Cid, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	return sm.callInternal(ctx, msg, nil, ts, stateCid, sm.GetNetworkVersion, true, execNoMessages, false)
}

// ApplyOnStateWithGasSkipSenderValidation applies the given message on top of the given state root
// with gas tracing enabled, but skips sender validation. This allows eth_call and eth_estimateGas
// to simulate calls from contract addresses or non-existent addresses, matching Geth's behavior.
func (sm *StateManager) ApplyOnStateWithGasSkipSenderValidation(ctx context.Context, stateCid cid.Cid, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	return sm.callInternal(ctx, msg, nil, ts, stateCid, sm.GetNetworkVersion, true, execNoMessages, true)
}

// CallWithGas calculates the state for a given tipset, and then applies the given message on top of that state.
func (sm *StateManager) CallWithGas(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet, applyTsMessages bool) (*api.InvocResult, error) {
	var strategy execMessageStrategy
	if applyTsMessages {
		strategy = execAllMessages
	} else {
		strategy = execSameSenderMessages
	}

	return sm.callInternal(ctx, msg, priorMsgs, ts, cid.Undef, sm.GetNetworkVersion, true, strategy, false)
}

// CallWithGasSkipSenderValidation is like CallWithGas but skips sender validation,
// creating a synthetic EthAccount actor if the sender doesn't exist on chain.
// This enables eth_estimateGas to work with non-existent addresses, matching Geth's behavior.
func (sm *StateManager) CallWithGasSkipSenderValidation(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet, applyTsMessages bool) (*api.InvocResult, error) {
	var strategy execMessageStrategy
	if applyTsMessages {
		strategy = execAllMessages
	} else {
		strategy = execSameSenderMessages
	}

	return sm.callInternal(ctx, msg, priorMsgs, ts, cid.Undef, sm.GetNetworkVersion, true, strategy, true)
}

// CallAtStateAndVersion allows you to specify a message to execute on the given stateCid and network version.
// This should mostly be used for gas modelling on a migrated state.
// Tipset here is not needed because stateCid and network version fully describe execution we want. The internal function
// will get the heaviest tipset for use for things like basefee, which we don't really care about here.
func (sm *StateManager) CallAtStateAndVersion(ctx context.Context, msg *types.Message, stateCid cid.Cid, v network.Version) (*api.InvocResult, error) {
	nvGetter := func(context.Context, abi.ChainEpoch) network.Version {
		return v
	}
	return sm.callInternal(ctx, msg, nil, nil, stateCid, nvGetter, true, execSameSenderMessages, false)
}

//   - If no tipset is specified, the first tipset without an expensive migration or one in its parent is used.
//   - If executing a message at a given tipset or its parent would trigger an expensive migration, the call will
//     fail with ErrExpensiveFork.
//   - If skipSenderValidation is true, the sender actor doesn't need to exist or be an account actor.
//     This enables eth_call/eth_estimateGas to simulate calls from contract addresses or non-existent addresses.
func (sm *StateManager) callInternal(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet, stateCid cid.Cid,
	nvGetter rand.NetworkVersionGetter, checkGas bool, strategy execMessageStrategy, skipSenderValidation bool) (*api.InvocResult, error) {
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
			if !sm.HasExpensiveForkBetween(pts.Height(), ts.Height()+1) {
				break
			}

			ts = pts
		}
	} else if ts.Height() > 0 {
		pts, err = sm.cs.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("failed to find a non-forking epoch: %w", err)
		}
		if sm.HasExpensiveForkBetween(pts.Height(), ts.Height()+1) {
			return nil, ErrExpensiveFork
		}
	}

	// Unless executing on a specific state cid, apply all the messages from the current tipset
	// first. Unfortunately, we can't just execute the tipset, because that will run cron. We
	// don't want to apply miner messages after cron runs in a given epoch.
	if stateCid == cid.Undef {
		stateCid = ts.ParentState()
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

	switch strategy {
	case execNoMessages:
		// Do nothing
	case execAllMessages, execSameSenderMessages:
		tsMsgs, err := sm.cs.MessagesForTipset(ctx, ts)
		if err != nil {
			return nil, xerrors.Errorf("failed to lookup messages for parent tipset: %w", err)
		}
		if strategy == execAllMessages {
			priorMsgs = append(tsMsgs, priorMsgs...)
		} else if strategy == execSameSenderMessages {
			var filteredTsMsgs []types.ChainMsg
			for _, tsMsg := range tsMsgs {
				//TODO we should technically be normalizing the filecoin address of from when we compare here
				if tsMsg.VMMessage().From == msg.VMMessage().From {
					filteredTsMsgs = append(filteredTsMsgs, tsMsg)
				}
			}
			priorMsgs = append(filteredTsMsgs, priorMsgs...)
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
	}

	stTree, err := state.LoadStateTree(cbor.NewCborStore(buffStore), stateCid)
	if err != nil {
		return nil, xerrors.Errorf("loading state tree: %w", err)
	}

	fromActor, err := stTree.GetActor(msg.From)
	if err != nil {
		if !skipSenderValidation {
			// Wrap as ErrSenderValidationFailed so callers can distinguish sender validation
			// failures from other errors and potentially retry with skip-sender-validation.
			return nil, &api.ErrSenderValidationFailed{
				Address: msg.From.String(),
				Reason:  fmt.Sprintf("actor not found: %v", err),
			}
		}
		// When skipping sender validation (for eth_call/eth_estimateGas simulation),
		// create a synthetic actor if the sender doesn't exist. This allows simulating
		// calls from contract addresses or non-existent addresses, matching Geth's behavior.
		fromActor, stateCid, vmi, err = sm.createSyntheticSenderActor(ctx, stTree, msg.From, ts, nvGetter, vmopt)
		if err != nil {
			return nil, err
		}
	} else if skipSenderValidation {
		// Actor exists, but we need to check if it's a valid sender type.
		// For contracts (EVM actors), we need to temporarily change the code to EthAccount
		// so the FVM accepts it as a valid sender during simulation.
		var newVmi vm.Interface
		fromActor, stateCid, newVmi, err = sm.maybeModifySenderForSimulation(ctx, stTree, fromActor, msg.From, ts, nvGetter, vmopt)
		if err != nil {
			return nil, err
		}
		if newVmi != nil {
			vmi = newVmi
		}
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
		var fromKey address.Address
		var err error
		if skipSenderValidation {
			// Skip resolving sender address since it may not exist on chain
			fromKey = msg.From
		} else {
			fromKey, err = sm.ResolveToDeterministicAddress(ctx, msg.From, ts)
			if err != nil {
				return nil, xerrors.Errorf("could not resolve key: %w", err)
			}
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

		ret, err = vmi.ApplyMessageSkipSenderValidation(ctx, msgApply)
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
	if err != nil && !errors.Is(err, errHaltExecution) {
		return nil, nil, xerrors.Errorf("unexpected error during execution: %w", err)
	}

	if finder.outr == nil {
		return nil, nil, xerrors.Errorf("given message not found in tipset")
	}

	return finder.outm, finder.outr, nil
}

// createSyntheticSenderActor creates a synthetic EthAccount actor for simulation when the sender
// address doesn't exist on chain. This enables eth_call/eth_estimateGas to work with non-existent
// addresses, matching Geth's behavior.
//
// IMPORTANT: State modifications are isolated via the buffered blockstore - changes are NOT
// persisted to the underlying store. The VM operates on a copy of state that is discarded after
// the simulation completes. This ensures eth_call simulations don't affect actual chain state.
//
// The synthetic actor is created with:
//   - Code: EthAccount (allows it to be a valid sender)
//   - Nonce: 0
//   - Balance: 0 (value transfers will fail as expected)
func (sm *StateManager) createSyntheticSenderActor(
	ctx context.Context,
	stTree *state.StateTree,
	fromAddr address.Address,
	ts *types.TipSet,
	nvGetter rand.NetworkVersionGetter,
	vmopt *vm.VMOpts,
) (*types.Actor, cid.Cid, vm.Interface, error) {
	log.Debugw("creating synthetic sender actor for simulation",
		"address", fromAddr,
		"height", ts.Height())

	nv := nvGetter(ctx, ts.Height())
	av, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to get actors version for network version %d: %w", nv, err)
	}

	ethAcctCid, ok := actors.GetActorCodeID(av, manifest.EthAccountKey)
	if !ok {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to get EthAccount actor code ID for actors version %d", av)
	}

	// Create synthetic actor with zero balance so simulations mirror Geth behavior
	// for non-existent senders (insufficient funds when gas/value is non-zero).
	syntheticActor := &types.Actor{
		Code:    ethAcctCid,
		Head:    vm.EmptyObjectCid,
		Nonce:   0,
		Balance: types.NewInt(0),
	}

	// Register the address with the Init actor to get an ID address
	idAddr, err := stTree.RegisterNewAddress(fromAddr)
	if err != nil {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to register synthetic actor address: %w", err)
	}

	// Add the synthetic actor to the state tree at the ID address
	if err := stTree.SetActor(idAddr, syntheticActor); err != nil {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to set synthetic actor in state tree: %w", err)
	}

	// Flush the state tree to get the new state root with the synthetic actor.
	// Note: This flush is to the buffered blockstore, NOT the underlying chain store.
	// All changes are ephemeral and will be discarded after the simulation.
	newStateCid, err := stTree.Flush(ctx)
	if err != nil {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to flush state tree with synthetic actor: %w", err)
	}

	vmopt.StateBase = newStateCid
	vmi, err := sm.newVM(ctx, vmopt)
	if err != nil {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to set up vm with synthetic actor: %w", err)
	}

	log.Debugw("synthetic sender actor created successfully",
		"address", fromAddr,
		"idAddress", idAddr,
		"newStateCid", newStateCid)

	return syntheticActor, newStateCid, vmi, nil
}

// maybeModifySenderForSimulation checks if the existing sender actor needs to be modified for
// simulation. Contract actors (EVM) need their code temporarily changed to EthAccount so the FVM
// accepts them as valid senders. Returns the (possibly modified) actor and updated VM state.
func (sm *StateManager) maybeModifySenderForSimulation(
	ctx context.Context,
	stTree *state.StateTree,
	fromActor *types.Actor,
	fromAddr address.Address,
	ts *types.TipSet,
	nvGetter rand.NetworkVersionGetter,
	vmopt *vm.VMOpts,
) (*types.Actor, cid.Cid, vm.Interface, error) {
	nv := nvGetter(ctx, ts.Height())
	av, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to get actors version for network version %d: %w", nv, err)
	}

	// Check if this is already a valid sender type (EthAccount or Account)
	ethAcctCid, _ := actors.GetActorCodeID(av, manifest.EthAccountKey)
	acctCid, _ := actors.GetActorCodeID(av, manifest.AccountKey)

	if fromActor.Code == ethAcctCid || fromActor.Code == acctCid {
		// Actor is already a valid sender, no modification needed
		return fromActor, vmopt.StateBase, nil, nil
	}

	// Only allow modification for specific actor types that make sense for simulation.
	// EVM actors (contracts) are the primary use case - simulating calls from contracts.
	// Placeholder actors may also need modification in some edge cases.
	evmCid, _ := actors.GetActorCodeID(av, manifest.EvmKey)
	placeholderCid, _ := actors.GetActorCodeID(av, manifest.PlaceholderKey)

	if fromActor.Code != evmCid && fromActor.Code != placeholderCid {
		// Actor is not an EVM/Placeholder type - don't allow modification.
		// This prevents simulation from system actors, payment channels, etc.
		return nil, cid.Undef, nil, xerrors.Errorf(
			"actor type cannot be used as sender for simulation (code=%s, address=%s): only EthAccount, Account, EVM, and Placeholder actors are supported",
			fromActor.Code, fromAddr)
	}

	// The actor is an EVM actor (contract) or Placeholder.
	// Create a modified version with EthAccount code for simulation.
	// Preserve the existing balance to keep simulation semantics intact.
	modifiedActor := &types.Actor{
		Code:    ethAcctCid,
		Head:    vm.EmptyObjectCid,
		Nonce:   fromActor.Nonce,
		Balance: fromActor.Balance,
	}

	// Look up the ID address for this actor
	idAddr, err := stTree.LookupIDAddress(fromAddr)
	if err != nil {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to lookup ID address: %w", err)
	}

	// Update the actor in the state tree
	if err := stTree.SetActor(idAddr, modifiedActor); err != nil {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to set modified actor in state tree: %w", err)
	}

	// Flush the state tree
	newStateCid, err := stTree.Flush(ctx)
	if err != nil {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to flush state tree with modified actor: %w", err)
	}

	vmopt.StateBase = newStateCid
	vmi, err := sm.newVM(ctx, vmopt)
	if err != nil {
		return nil, cid.Undef, nil, xerrors.Errorf("failed to set up vm with modified actor: %w", err)
	}

	return modifiedActor, newStateCid, vmi, nil
}
