package vm

import (
	"context"

	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/chain/actors"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	/* inline-gen template
	   {{range .actorVersions}}
	   	builtin{{.}} "github.com/filecoin-project/specs-actors{{import .}}actors/builtin"{{end}}

	/* inline-gen start */

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	/* inline-gen end */

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/account"
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

// TryCreateAccountActor creates account actors from only BLS/SECP256K1 addresses.
func TryCreateAccountActor(rt *Runtime, addr address.Address) (*types.Actor, address.Address, aerrors.ActorError) {
	if err := rt.chargeGasSafe(PricelistByEpoch(rt.height).OnCreateActor()); err != nil {
		return nil, address.Undef, err
	}

	if addr == build.ZeroAddress && rt.NetworkVersion() >= network.Version10 {
		return nil, address.Undef, aerrors.New(exitcode.ErrIllegalArgument, "cannot create the zero bls actor")
	}

	addrID, err := rt.state.RegisterNewAddress(addr)
	if err != nil {
		return nil, address.Undef, aerrors.Escalate(err, "registering actor address")
	}

	av, err := actors.VersionForNetwork(rt.NetworkVersion())
	if err != nil {
		return nil, address.Undef, aerrors.Escalate(err, "unsupported network version")
	}

	act, aerr := makeAccountActor(av, addr)
	if aerr != nil {
		return nil, address.Undef, aerr
	}

	if err := rt.state.SetActor(addrID, act); err != nil {
		return nil, address.Undef, aerrors.Escalate(err, "creating new actor failed")
	}

	p, err := actors.SerializeParams(&addr)
	if err != nil {
		return nil, address.Undef, aerrors.Escalate(err, "couldn't serialize params for actor construction")
	}
	// call constructor on account

	_, aerr = rt.internalSend(builtin.SystemActorAddr, addrID, account.Methods.Constructor, big.Zero(), p)
	if aerr != nil {
		return nil, address.Undef, aerrors.Wrap(aerr, "failed to invoke account constructor")
	}

	act, err = rt.state.GetActor(addrID)
	if err != nil {
		return nil, address.Undef, aerrors.Escalate(err, "loading newly created actor failed")
	}
	return act, addrID, nil
}

func makeAccountActor(ver actors.Version, addr address.Address) (*types.Actor, aerrors.ActorError) {
	switch addr.Protocol() {
	case address.BLS, address.SECP256K1:
		return newAccountActor(ver), nil
	case address.ID:
		return nil, aerrors.Newf(exitcode.SysErrInvalidReceiver, "no actor with given ID: %s", addr)
	case address.Actor:
		return nil, aerrors.Newf(exitcode.SysErrInvalidReceiver, "no such actor: %s", addr)
	default:
		return nil, aerrors.Newf(exitcode.SysErrInvalidReceiver, "address has unsupported protocol: %d", addr.Protocol())
	}
}

func newAccountActor(ver actors.Version) *types.Actor {
	// TODO: ActorsUpgrade use a global actor registry?
	var code cid.Cid
	switch ver {
	/* inline-gen template
	   {{range .actorVersions}}
		case actors.Version{{.}}:
			code = builtin{{.}}.AccountActorCodeID{{end}}
	/* inline-gen start */

	case actors.Version0:
		code = builtin0.AccountActorCodeID
	case actors.Version2:
		code = builtin2.AccountActorCodeID
	case actors.Version3:
		code = builtin3.AccountActorCodeID
	case actors.Version4:
		code = builtin4.AccountActorCodeID
	case actors.Version5:
		code = builtin5.AccountActorCodeID
	case actors.Version6:
		code = builtin6.AccountActorCodeID
		/* inline-gen end */
	default:
		panic("unsupported actors version")
	}
	nact := &types.Actor{
		Code:    code,
		Balance: types.NewInt(0),
		Head:    EmptyObjectCid,
	}

	return nact
}
