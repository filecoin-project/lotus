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
		return nil, aerrors.Escalate(err, "registering actor address")
	}

	act, aerr := makeActor(addr)
	if aerr != nil {
		return nil, aerr
	}

	if err := rt.state.SetActor(addrID, act); err != nil {
		return nil, aerrors.Escalate(err, "creating new actor failed")
	}

	p, err := actors.SerializeParams(&addr)
	if err != nil {
		return nil, aerrors.Escalate(err, "couldn't serialize params for actor construction")
	}
	// call constructor on account

	if err := rt.chargeGasSafe(PricelistByEpoch(rt.height).OnCreateActor()); err != nil {
		return nil, err
	}

	_, aerr = rt.internalSend(builtin.SystemActorAddr, addrID, builtin.MethodsAccount.Constructor, big.Zero(), p)
	if aerr != nil {
		return nil, aerrors.Wrap(aerr, "failed to invoke account constructor")
	}

	act, err = rt.state.GetActor(addrID)
	if err != nil {
		return nil, aerrors.Escalate(err, "loading newly created actor failed")
	}
	return act, nil
}

func makeActor(addr address.Address) (*types.Actor, aerrors.ActorError) {
	switch addr.Protocol() {
	case address.BLS:
		return NewBLSAccountActor(), nil
	case address.SECP256K1:
		return NewSecp256k1AccountActor(), nil
	case address.ID:
		return nil, aerrors.Newf(exitcode.SysErrInvalidReceiver, "no actor with given ID: %s", addr)
	case address.Actor:
		return nil, aerrors.Newf(exitcode.SysErrInvalidReceiver, "no such actor: %s", addr)
	default:
		return nil, aerrors.Newf(exitcode.SysErrInvalidReceiver, "address has unsupported protocol: %d", addr.Protocol())
	}
}

func NewBLSAccountActor() *types.Actor {
	nact := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Balance: types.NewInt(0),
		Head:    EmptyObjectCid,
	}

	return nact
}

func NewSecp256k1AccountActor() *types.Actor {
	nact := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Balance: types.NewInt(0),
		Head:    EmptyObjectCid,
	}

	return nact
}
