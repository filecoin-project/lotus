package stmgr

import (
	"context"
	"sync"

	amt "github.com/filecoin-project/go-amt-ipld"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("statemgr")

type StateManager struct {
	cs *store.ChainStore

	stCache map[string][]cid.Cid
	stlk    sync.Mutex
}

func NewStateManager(cs *store.ChainStore) *StateManager {
	return &StateManager{
		cs:      cs,
		stCache: make(map[string][]cid.Cid),
	}
}

func cidsToKey(cids []cid.Cid) string {
	var out string
	for _, c := range cids {
		out += c.KeyString()
	}
	return out
}

func (sm *StateManager) TipSetState(cids []cid.Cid) (cid.Cid, cid.Cid, error) {
	ctx := context.TODO()

	ck := cidsToKey(cids)
	sm.stlk.Lock()
	cached, ok := sm.stCache[ck]
	sm.stlk.Unlock()
	if ok {
		return cached[0], cached[1], nil
	}

	ts, err := sm.cs.LoadTipSet(cids)
	if err != nil {
		return cid.Undef, cid.Undef, err
	}

	st, rec, err := sm.computeTipSetState(ctx, ts.Blocks(), nil)
	if err != nil {
		return cid.Undef, cid.Undef, err
	}

	sm.stlk.Lock()
	sm.stCache[ck] = []cid.Cid{st, rec}
	sm.stlk.Unlock()
	return st, rec, nil
}

type ChainMsg interface {
	Cid() cid.Cid
	VMMessage() *types.Message
}

func (sm *StateManager) computeTipSetState(ctx context.Context, blks []*types.BlockHeader, cb func(cid.Cid, *types.Message, *vm.ApplyRet) error) (cid.Cid, cid.Cid, error) {
	pstate := blks[0].ParentStateRoot

	cids := make([]cid.Cid, len(blks))
	for i, v := range blks {
		cids[i] = v.Cid()
	}

	r := vm.NewChainRand(sm.cs, cids, blks[0].Height, nil)

	vmi, err := vm.NewVM(pstate, blks[0].Height, r, address.Undef, sm.cs)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("instantiating VM failed: %w", err)
	}

	/* TODO: apply mining reward
	netbalance, err := vmi.ActorBalance(actors.NetworkAddress)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get network actor balance: %w", err)
	}

	vm.MiningRewardForBlock(netbalance)
	*/

	applied := make(map[address.Address]uint64)
	balances := make(map[address.Address]types.BigInt)

	preloadAddr := func(a address.Address) error {
		if _, ok := applied[a]; !ok {
			act, err := vmi.StateTree().GetActor(a)
			if err != nil {
				return err
			}

			applied[a] = act.Nonce
			balances[a] = act.Balance
		}
		return nil
	}

	var receipts []cbg.CBORMarshaler
	for _, b := range blks {
		vmi.SetBlockMiner(b.Miner)

		bms, sms, err := sm.cs.MessagesForBlock(b)
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("failed to get messages for block: %w", err)
		}

		cmsgs := make([]ChainMsg, 0, len(bms)+len(sms))
		for _, m := range bms {
			cmsgs = append(cmsgs, m)
		}
		for _, sm := range sms {
			cmsgs = append(cmsgs, sm)
		}

		for _, cm := range cmsgs {
			m := cm.VMMessage()
			if err := preloadAddr(m.From); err != nil {
				return cid.Undef, cid.Undef, err
			}

			if applied[m.From] != m.Nonce {
				continue
			}
			applied[m.From]++

			if balances[m.From].LessThan(m.RequiredFunds()) {
				continue
			}
			balances[m.From] = types.BigSub(balances[m.From], m.RequiredFunds())

			r, err := vmi.ApplyMessage(ctx, m)
			if err != nil {
				return cid.Undef, cid.Undef, err
			}

			receipts = append(receipts, &r.MessageReceipt)

			if cb != nil {
				if err := cb(cm.Cid(), m, r); err != nil {
					return cid.Undef, cid.Undef, err
				}
			}
		}
	}

	bs := amt.WrapBlockstore(sm.cs.Blockstore())
	rectroot, err := amt.FromArray(bs, receipts)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("failed to build receipts amt: %w", err)
	}

	st, err := vmi.Flush(ctx)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("vm flush failed: %w", err)
	}

	return st, rectroot, nil
}

func (sm *StateManager) GetActor(addr address.Address, ts *types.TipSet) (*types.Actor, error) {
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	stcid, _, err := sm.TipSetState(ts.Cids())
	if err != nil {
		return nil, xerrors.Errorf("tipset state: %w", err)
	}

	cst := hamt.CSTFromBstore(sm.cs.Blockstore())
	state, err := state.LoadStateTree(cst, stcid)
	if err != nil {
		return nil, xerrors.Errorf("load state tree: %w", err)
	}

	return state.GetActor(addr)
}

func (sm *StateManager) GetBalance(addr address.Address, ts *types.TipSet) (types.BigInt, error) {
	act, err := sm.GetActor(addr, ts)
	if err != nil {
		return types.BigInt{}, xerrors.Errorf("get actor: %w", err)
	}

	return act.Balance, nil
}

func (sm *StateManager) ChainStore() *store.ChainStore {
	return sm.cs
}

func (sm *StateManager) LoadActorState(ctx context.Context, a address.Address, out interface{}, ts *types.TipSet) (*types.Actor, error) {
	act, err := sm.GetActor(a, ts)
	if err != nil {
		return nil, err
	}

	cst := hamt.CSTFromBstore(sm.cs.Blockstore())
	if err := cst.Get(ctx, act.Head, out); err != nil {
		return nil, err
	}

	return act, nil
}
