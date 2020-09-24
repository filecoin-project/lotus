package init

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin1 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
)

func init() {
	builtin.RegisterActorState(builtin0.InitActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load0(store, root)
	})
	builtin.RegisterActorState(builtin1.InitActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load1(store, root)
	})
}

var Address = builtin0.InitActorAddr

func Load(store adt.Store, act *types.Actor) (State, error) {
	switch act.Code {
	case builtin0.InitActorCodeID:
		return load0(store, act.Head)
	case builtin1.InitActorCodeID:
		return load1(store, act.Head)
	}
	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

type State interface {
	cbor.Marshaler

	ResolveAddress(address address.Address) (address.Address, bool, error)
	MapAddressToNewID(address address.Address) (address.Address, error)
	NetworkName() (dtypes.NetworkName, error)

	ForEachActor(func(id abi.ActorID, address address.Address) error) error

	// Remove exists to support tooling that manipulates state for testing.
	// It should not be used in production code, as init actor entries are
	// immutable.
	Remove(addrs ...address.Address) error
}
