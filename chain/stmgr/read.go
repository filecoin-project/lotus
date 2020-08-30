package stmgr

import (
	"context"
	"reflect"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

type StateTreeCB func(state *state.StateTree) error

func (sm *StateManager) WithParentStateTsk(tsk types.TipSetKey, cb StateTreeCB) error {
	ts, err := sm.cs.GetTipSetFromKey(tsk)
	if err != nil {
		return xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	cst := cbor.NewCborStore(sm.cs.Blockstore())
	state, err := state.LoadStateTree(cst, sm.parentState(ts))
	if err != nil {
		return xerrors.Errorf("load state tree: %w", err)
	}

	return cb(state)
}

func (sm *StateManager) WithParentState(ts *types.TipSet, cb StateTreeCB) error {
	cst := cbor.NewCborStore(sm.cs.Blockstore())
	state, err := state.LoadStateTree(cst, sm.parentState(ts))
	if err != nil {
		return xerrors.Errorf("load state tree: %w", err)
	}

	return cb(state)
}

func (sm *StateManager) WithStateTree(st cid.Cid, cb StateTreeCB) error {
	cst := cbor.NewCborStore(sm.cs.Blockstore())
	state, err := state.LoadStateTree(cst, st)
	if err != nil {
		return xerrors.Errorf("load state tree: %w", err)
	}

	return cb(state)
}

type ActorCB func(act *types.Actor) error

func GetActor(out *types.Actor) ActorCB {
	return func(act *types.Actor) error {
		*out = *act
		return nil
	}
}

func (sm *StateManager) WithActor(addr address.Address, cb ActorCB) StateTreeCB {
	return func(state *state.StateTree) error {
		act, err := state.GetActor(addr)
		if err != nil {
			return xerrors.Errorf("get actor: %w", err)
		}

		return cb(act)
	}
}

// WithActorState usage:
// Option 1: WithActorState(ctx, idAddr, func(store adt.Store, st *ActorStateType) error {...})
// Option 2: WithActorState(ctx, idAddr, actorStatePtr)
func (sm *StateManager) WithActorState(ctx context.Context, out interface{}) ActorCB {
	return func(act *types.Actor) error {
		store := sm.cs.Store(ctx)

		outCallback := reflect.TypeOf(out).Kind() == reflect.Func

		var st reflect.Value
		if outCallback {
			st = reflect.New(reflect.TypeOf(out).In(1).Elem())
		} else {
			st = reflect.ValueOf(out)
		}
		if err := store.Get(ctx, act.Head, st.Interface()); err != nil {
			return xerrors.Errorf("read actor head: %w", err)
		}

		if outCallback {
			out := reflect.ValueOf(out).Call([]reflect.Value{reflect.ValueOf(store), st})
			if !out[0].IsNil() && out[0].Interface().(error) != nil {
				return out[0].Interface().(error)
			}
		}

		return nil
	}
}

type DeadlinesCB func(store adt.Store, deadlines *miner.Deadlines) error

func (sm *StateManager) WithDeadlines(cb DeadlinesCB) func(store adt.Store, mas *miner.State) error {
	return func(store adt.Store, mas *miner.State) error {
		deadlines, err := mas.LoadDeadlines(store)
		if err != nil {
			return err
		}

		return cb(store, deadlines)
	}
}

type DeadlineCB func(store adt.Store, idx uint64, deadline *miner.Deadline) error

func (sm *StateManager) WithDeadline(idx uint64, cb DeadlineCB) DeadlinesCB {
	return func(store adt.Store, deadlines *miner.Deadlines) error {
		d, err := deadlines.LoadDeadline(store, idx)
		if err != nil {
			return err
		}

		return cb(store, idx, d)
	}
}

func (sm *StateManager) WithEachDeadline(cb DeadlineCB) DeadlinesCB {
	return func(store adt.Store, deadlines *miner.Deadlines) error {
		return deadlines.ForEach(store, func(dlIdx uint64, dl *miner.Deadline) error {
			return cb(store, dlIdx, dl)
		})
	}
}

type PartitionCB func(store adt.Store, idx uint64, partition *miner.Partition) error

func (sm *StateManager) WithEachPartition(cb PartitionCB) DeadlineCB {
	return func(store adt.Store, idx uint64, deadline *miner.Deadline) error {
		parts, err := deadline.PartitionsArray(store)
		if err != nil {
			return err
		}

		var partition miner.Partition
		return parts.ForEach(&partition, func(i int64) error {
			p := partition
			return cb(store, uint64(i), &p)
		})
	}
}
