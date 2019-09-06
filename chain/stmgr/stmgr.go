package stmgr

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("chainstore")

type StateManager struct {
	cs *store.ChainStore
}

func NewStateManager(cs *store.ChainStore) *StateManager {
	return &StateManager{cs}
}

func (sm *StateManager) TipSetState(cids []cid.Cid) (cid.Cid, error) {
	ctx := context.TODO()

	ts, err := sm.cs.LoadTipSet(cids)
	if err != nil {
		log.Error("failed loading tipset: ", cids)
		return cid.Undef, err
	}

	if len(ts.Blocks()) == 1 {
		return ts.Blocks()[0].StateRoot, nil
	}

	pstate, err := sm.TipSetState(ts.Parents())
	if err != nil {
		return cid.Undef, err
	}

	vmi, err := vm.NewVM(pstate, ts.Height(), address.Undef, sm.cs)
	if err != nil {
		return cid.Undef, err
	}

	applied := make(map[cid.Cid]bool)
	for _, b := range ts.Blocks() {
		vmi.SetBlockMiner(b.Miner)

		bms, sms, err := sm.cs.MessagesForBlock(b)
		if err != nil {
			return cid.Undef, err
		}

		for _, m := range bms {
			if applied[m.Cid()] {
				continue
			}
			applied[m.Cid()] = true

			_, err := vmi.ApplyMessage(ctx, m)
			if err != nil {
				return cid.Undef, err
			}
		}

		for _, sm := range sms {
			if applied[sm.Cid()] {
				continue
			}
			applied[sm.Cid()] = true

			_, err := vmi.ApplyMessage(ctx, &sm.Message)
			if err != nil {
				return cid.Undef, err
			}
		}
	}

	return vmi.Flush(ctx)
}

func (sm *StateManager) GetActor(addr address.Address) (*types.Actor, error) {
	ts := sm.cs.GetHeaviestTipSet()
	stcid, err := sm.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	cst := hamt.CSTFromBstore(sm.cs.Blockstore())
	state, err := state.LoadStateTree(cst, stcid)
	if err != nil {
		return nil, err
	}

	return state.GetActor(addr)
}

func (sm *StateManager) GetBalance(addr address.Address) (types.BigInt, error) {
	act, err := sm.GetActor(addr)
	if err != nil {
		return types.BigInt{}, err
	}

	return act.Balance, nil
}

func (sm *StateManager) ChainStore() *store.ChainStore {
	return sm.cs
}

func (sm *StateManager) LoadActorState(ctx context.Context, a address.Address, out interface{}) (*types.Actor, error) {
	act, err := sm.GetActor(a)
	if err != nil {
		return nil, err
	}

	cst := hamt.CSTFromBstore(sm.cs.Blockstore())
	if err := cst.Get(ctx, act.Head, out); err != nil {
		return nil, err
	}

	return act, nil

}
