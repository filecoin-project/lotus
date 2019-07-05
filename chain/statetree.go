package chain


import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-lotus/chain/address"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
)

var ErrActorNotFound = fmt.Errorf("actor not found")

type StateTree struct {
	root  *hamt.Node
	store *hamt.CborIpldStore

	actorcache map[address.Address]*Actor
	snapshot   cid.Cid
}

func NewStateTree(cst *hamt.CborIpldStore) (*StateTree, error) {
	return &StateTree{
		root:       hamt.NewNode(cst),
		store:      cst,
		actorcache: make(map[address.Address]*Actor),
	}, nil
}

func LoadStateTree(cst *hamt.CborIpldStore, c cid.Cid) (*StateTree, error) {
	nd, err := hamt.LoadNode(context.Background(), cst, c)
	if err != nil {
		log.Errorf("loading hamt node failed: %s", err)
		return nil, err
	}

	return &StateTree{
		root:       nd,
		store:      cst,
		actorcache: make(map[address.Address]*Actor),
	}, nil
}

func (st *StateTree) SetActor(addr address.Address, act *Actor) error {
	if addr.Protocol() != address.ID {
		iaddr, err := st.lookupID(addr)
		if err != nil {
			return err
		}
		addr = iaddr
	}

	cact, ok := st.actorcache[addr]
	if ok {
		if act == cact {
			return nil
		}
	}

	return st.root.Set(context.TODO(), string(addr.Bytes()), act)
}

func (st *StateTree) lookupID(addr address.Address) (address.Address, error) {
	act, err := st.GetActor(InitActorAddress)
	if err != nil {
		return address.Undef, err
	}

	var ias InitActorState
	if err := st.store.Get(context.TODO(), act.Head, &ias); err != nil {
		return address.Undef, err
	}

	return ias.Lookup(st.store, addr)
}

func (st *StateTree) GetActor(addr address.Address) (*Actor, error) {
	if addr.Protocol() != address.ID {
		iaddr, err := st.lookupID(addr)
		if err != nil {
			if err == hamt.ErrNotFound {
				return nil, ErrActorNotFound
			}
			return nil, err
		}
		addr = iaddr
	}

	cact, ok := st.actorcache[addr]
	if ok {
		return cact, nil
	}

	thing, err := st.root.Find(context.TODO(), string(addr.Bytes()))
	if err != nil {
		if err == hamt.ErrNotFound {
			return nil, ErrActorNotFound
		}
		return nil, err
	}

	var act Actor
	badout, err := cbor.DumpObject(thing)
	if err != nil {
		return nil, err
	}

	if err := cbor.DecodeInto(badout, &act); err != nil {
		return nil, err
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
	st.actorcache = make(map[address.Address]*Actor)

	if err := st.root.Flush(context.TODO()); err != nil {
		return cid.Undef, err
	}

	return st.store.Put(context.TODO(), st.root)
}

func (st *StateTree) Snapshot() error {
	ss, err := st.Flush()
	if err != nil {
		return err
	}

	st.snapshot = ss
	return nil
}

func (st *StateTree) RegisterNewAddress(addr address.Address, act *Actor) (address.Address, error) {
	var out address.Address
	err := st.MutateActor(InitActorAddress, func(initact *Actor) error {
		var ias InitActorState
		if err := st.store.Get(context.TODO(), initact.Head, &ias); err != nil {
			return err
		}

		fvm := &VMContext{cst: st.store}
		oaddr, err := ias.AddActor(fvm, addr)
		if err != nil {
			return err
		}
		out = oaddr

		ncid, err := st.store.Put(context.TODO(), &ias)
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
	nd, err := hamt.LoadNode(context.Background(), st.store, st.snapshot)
	if err != nil {
		return err
	}

	st.root = nd
	return nil
}

func (st *StateTree) MutateActor(addr address.Address, f func(*Actor) error) error {
	act, err := st.GetActor(addr)
	if err != nil {
		return err
	}

	if err := f(act); err != nil {
		return err
	}

	return st.SetActor(addr, act)
}

func NewBLSAccountActor(st *StateTree, addr address.Address) (*Actor, error) {
	var acstate AccountActorState
	acstate.Address = addr

	c, err := st.store.Put(context.TODO(), acstate)
	if err != nil {
		return nil, err
	}

	nact := &Actor{
		Code:    AccountActorCodeCid,
		Balance: NewInt(0),
		Head:    c,
	}

	return nact, nil
}

func NewSecp256k1AccountActor(st *StateTree, addr address.Address) (*Actor, error) {
	nact := &Actor{
		Code:    AccountActorCodeCid,
		Balance: NewInt(0),
		Head:    EmptyObjectCid,
	}

	return nact, nil
}

func TryCreateAccountActor(st *StateTree, addr address.Address) (*Actor, error) {
	act, err := makeActor(st, addr)
	if err != nil {
		return nil, err
	}

	_, err = st.RegisterNewAddress(addr, act)
	if err != nil {
		return nil, err
	}

	return act, nil
}

func makeActor(st *StateTree, addr address.Address) (*Actor, error) {
	switch addr.Protocol() {
	case address.BLS:
		return NewBLSAccountActor(st, addr)
	case address.SECP256K1:
		return NewSecp256k1AccountActor(st, addr)
	case address.ID:
		return nil, fmt.Errorf("no actor with given ID")
	case address.Actor:
		return nil, fmt.Errorf("no such actor")
	default:
		return nil, fmt.Errorf("address has unsupported protocol: %d", addr.Protocol())
	}
}
