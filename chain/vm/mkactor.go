package vm

import (
	"context"

	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
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

func TryCreateAccountActor(st *state.StateTree, addr address.Address) (*types.Actor, aerrors.ActorError) {
	act, err := makeActor(st, addr)
	if err != nil {
		return nil, err
	}

	if _, err := st.RegisterNewAddress(addr, act); err != nil {
		return nil, aerrors.Escalate(err, "registering actor address")
	}

	return act, nil
}

func makeActor(st *state.StateTree, addr address.Address) (*types.Actor, aerrors.ActorError) {
	switch addr.Protocol() {
	case address.BLS:
		return NewBLSAccountActor(st, addr)
	case address.SECP256K1:
		return NewSecp256k1AccountActor(st, addr)
	case address.ID:
		return nil, aerrors.Newf(1, "no actor with given ID: %s", addr)
	case address.Actor:
		return nil, aerrors.Newf(1, "no such actor: %s", addr)
	default:
		return nil, aerrors.Newf(1, "address has unsupported protocol: %d", addr.Protocol())
	}
}

func NewBLSAccountActor(st *state.StateTree, addr address.Address) (*types.Actor, aerrors.ActorError) {
	var acstate actors.AccountActorState
	acstate.Address = addr

	c, err := st.Store.Put(context.TODO(), &acstate)
	if err != nil {
		return nil, aerrors.Escalate(err, "serializing account actor state")
	}

	nact := &types.Actor{
		Code:    actors.AccountCodeCid,
		Balance: types.NewInt(0),
		Head:    c,
	}

	return nact, nil
}

func NewSecp256k1AccountActor(st *state.StateTree, addr address.Address) (*types.Actor, aerrors.ActorError) {
	nact := &types.Actor{
		Code:    actors.AccountCodeCid,
		Balance: types.NewInt(0),
		Head:    EmptyObjectCid,
	}

	return nact, nil
}
