package vm

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
)

func init() {
	bs := bstore.NewBlockstore(dstore.NewMapDatastore())
	cst := hamt.CSTFromBstore(bs)
	emptyobject, err := cst.Put(context.TODO(), map[string]string{})
	if err != nil {
		panic(err)
	}

	EmptyObjectCid = emptyobject
}

var EmptyObjectCid cid.Cid

func TryCreateAccountActor(st *state.StateTree, addr address.Address) (*types.Actor, error) {
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

func makeActor(st *state.StateTree, addr address.Address) (*types.Actor, error) {
	switch addr.Protocol() {
	case address.BLS:
		return NewBLSAccountActor(st, addr)
	case address.SECP256K1:
		return NewSecp256k1AccountActor(st, addr)
	case address.ID:
		return nil, fmt.Errorf("no actor with given ID")
	case address.Actor:
		return nil, fmt.Errorf("no such actor: %s", addr)
	default:
		return nil, fmt.Errorf("address has unsupported protocol: %d", addr.Protocol())
	}
}

func NewBLSAccountActor(st *state.StateTree, addr address.Address) (*types.Actor, error) {
	var acstate actors.AccountActorState
	acstate.Address = addr

	c, err := st.Store.Put(context.TODO(), acstate)
	if err != nil {
		return nil, err
	}

	nact := &types.Actor{
		Code:    actors.AccountActorCodeCid,
		Balance: types.NewInt(0),
		Head:    c,
	}

	return nact, nil
}

func NewSecp256k1AccountActor(st *state.StateTree, addr address.Address) (*types.Actor, error) {
	nact := &types.Actor{
		Code:    actors.AccountActorCodeCid,
		Balance: types.NewInt(0),
		Head:    EmptyObjectCid,
	}

	return nact, nil
}
