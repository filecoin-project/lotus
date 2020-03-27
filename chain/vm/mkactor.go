package vm

import (
	"context"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
)

func init() {
	cst := cbor.NewMemCborStore()
	emptyobject, err := cst.Put(context.TODO(), []struct{}{})
	if err != nil {
		panic(err)
	}

	EmptyObjectCid = emptyobject
}

var EmptyObjectCid cid.Cid

// Creates account actors from only BLS/SECP256K1 addresses.
func TryCreateAccountActor(rt *Runtime, addr address.Address) (*types.Actor, aerrors.ActorError) {
	addrID, err := rt.state.RegisterNewAddress(addr)
	if err != nil {
		return nil, aerrors.Absorb(err, byte(exitcode.SysErrInternal), "registering actor address")
	}

	if err := rt.chargeGasSafe(PricelistByEpoch(rt.height).OnCreateActor()); err != nil {
		return nil, err
	}

	act, aerr := makeActor(rt.state, addr)
	if aerr != nil {
		return nil, aerr
	}

	if err := rt.state.SetActor(addrID, act); err != nil {
		return nil, aerrors.Absorb(err, byte(exitcode.SysErrInternal), "creating new actor failed")
	}

	p, err := actors.SerializeParams(&addr)
	if err != nil {
		return nil, aerrors.Absorb(err, byte(exitcode.SysErrInternal), "registering actor address")
	}
	// call constructor on account

	_, aerr = rt.internalSend(builtin.SystemActorAddr, addrID, builtin.MethodsAccount.Constructor, big.Zero(), p)
	if aerr != nil {
		return nil, aerrors.Wrap(aerr, "failed to invoke account constructor")
	}

	act, err = rt.state.GetActor(addrID)
	if err != nil {
		return nil, aerrors.Absorb(err, byte(exitcode.SysErrInternal), "loading newly created actor failed")
	}
	return act, nil
}

func makeActor(st *state.StateTree, addr address.Address) (*types.Actor, aerrors.ActorError) {
	switch addr.Protocol() {
	case address.BLS:
		return NewBLSAccountActor()
	case address.SECP256K1:
		return NewSecp256k1AccountActor()
	case address.ID:
		return nil, aerrors.Newf(1, "no actor with given ID: %s", addr)
	case address.Actor:
		return nil, aerrors.Newf(1, "no such actor: %s", addr)
	default:
		return nil, aerrors.Newf(1, "address has unsupported protocol: %d", addr.Protocol())
	}
}

func NewBLSAccountActor() (*types.Actor, aerrors.ActorError) {
	nact := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Balance: types.NewInt(0),
		Head:    EmptyObjectCid,
	}

	return nact, nil
}

func NewSecp256k1AccountActor() (*types.Actor, aerrors.ActorError) {
	nact := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Balance: types.NewInt(0),
		Head:    EmptyObjectCid,
	}

	return nact, nil
}
