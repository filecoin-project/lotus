package state

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("statetree")

type StateTree struct {
	root  *hamt.Node
	Store *hamt.CborIpldStore

	actorcache map[address.Address]*types.Actor
	snapshot   cid.Cid
}

func NewStateTree(cst *hamt.CborIpldStore) (*StateTree, error) {
	return &StateTree{
		root:       hamt.NewNode(cst),
		Store:      cst,
		actorcache: make(map[address.Address]*types.Actor),
	}, nil
}

func LoadStateTree(cst *hamt.CborIpldStore, c cid.Cid) (*StateTree, error) {
	nd, err := hamt.LoadNode(context.Background(), cst, c)
	if err != nil {
		log.Errorf("loading hamt node %s failed: %s", c, err)
		return nil, err
	}

	return &StateTree{
		root:       nd,
		Store:      cst,
		actorcache: make(map[address.Address]*types.Actor),
	}, nil
}

func (st *StateTree) SetActor(addr address.Address, act *types.Actor) error {
	iaddr, err := st.LookupID(addr)
	if err != nil {
		return xerrors.Errorf("ID lookup failed: %w", err)
	}
	addr = iaddr

	cact, ok := st.actorcache[addr]
	if ok {
		if act == cact {
			return nil
		}
	}

	st.actorcache[addr] = act

	return st.root.Set(context.TODO(), string(addr.Bytes()), act)
}

func (st *StateTree) LookupID(addr address.Address) (address.Address, error) {
	if addr.Protocol() == address.ID {
		return addr, nil
	}

	act, err := st.GetActor(actors.InitAddress)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting init actor: %w", err)
	}

	var ias actors.InitActorState
	if err := st.Store.Get(context.TODO(), act.Head, &ias); err != nil {
		return address.Undef, xerrors.Errorf("loading init actor state: %w", err)
	}

	return ias.Lookup(st.Store, addr)
}

func (st *StateTree) GetActor(addr address.Address) (*types.Actor, error) {
	if addr == address.Undef {
		return nil, fmt.Errorf("GetActor called on undefined address")
	}

	iaddr, err := st.LookupID(addr)
	if err != nil {
		if xerrors.Is(err, hamt.ErrNotFound) {
			return nil, xerrors.Errorf("resolution lookup failed (%s): %w", addr, types.ErrActorNotFound)
		}
		return nil, xerrors.Errorf("address resolution: %w", err)
	}
	addr = iaddr

	cact, ok := st.actorcache[addr]
	if ok {
		return cact, nil
	}

	var act types.Actor
	err = st.root.Find(context.TODO(), string(addr.Bytes()), &act)
	if err != nil {
		if err == hamt.ErrNotFound {
			return nil, types.ErrActorNotFound
		}
		return nil, xerrors.Errorf("hamt find failed: %w", err)
	}

	st.actorcache[addr] = &act

	return &act, nil
}

func (st *StateTree) Flush() (cid.Cid, error) {
	for addr, act := range st.actorcache {
		if err := st.root.Set(context.TODO(), string(addr.Bytes()), act); err != nil {
			return cid.Undef, err
		}
	}
	st.actorcache = make(map[address.Address]*types.Actor)

	if err := st.root.Flush(context.TODO()); err != nil {
		return cid.Undef, err
	}

	return st.Store.Put(context.TODO(), st.root)
}

func (st *StateTree) Snapshot() error {
	ss, err := st.Flush()
	if err != nil {
		return err
	}

	st.snapshot = ss
	return nil
}

func (st *StateTree) RegisterNewAddress(addr address.Address, act *types.Actor) (address.Address, error) {
	var out address.Address
	err := st.MutateActor(actors.InitAddress, func(initact *types.Actor) error {
		var ias actors.InitActorState
		if err := st.Store.Get(context.TODO(), initact.Head, &ias); err != nil {
			return err
		}

		oaddr, err := ias.AddActor(st.Store, addr)
		if err != nil {
			return err
		}
		out = oaddr

		ncid, err := st.Store.Put(context.TODO(), &ias)
		if err != nil {
			return err
		}

		initact.Head = ncid
		return nil
	})
	if err != nil {
		return address.Undef, err
	}

	if err := st.SetActor(out, act); err != nil {
		return address.Undef, err
	}

	return out, nil
}

func (st *StateTree) Revert() error {
	nd, err := hamt.LoadNode(context.Background(), st.Store, st.snapshot)
	if err != nil {
		return err
	}

	st.root = nd
	return nil
}

func (st *StateTree) MutateActor(addr address.Address, f func(*types.Actor) error) error {
	act, err := st.GetActor(addr)
	if err != nil {
		return err
	}

	if err := f(act); err != nil {
		return err
	}

	return st.SetActor(addr, act)
}
