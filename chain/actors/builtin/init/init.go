package init

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin18 "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/manifest"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var (
	Address = builtin18.InitActorAddr
	Methods = builtin18.MethodsInit
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != manifest.InitKey {
			return nil, xerrors.Errorf("actor code is not init: %s", name)
		}

		switch av {

		case actorstypes.Version8:
			return load8(store, act.Head)

		case actorstypes.Version9:
			return load9(store, act.Head)

		case actorstypes.Version10:
			return load10(store, act.Head)

		case actorstypes.Version11:
			return load11(store, act.Head)

		case actorstypes.Version12:
			return load12(store, act.Head)

		case actorstypes.Version13:
			return load13(store, act.Head)

		case actorstypes.Version14:
			return load14(store, act.Head)

		case actorstypes.Version15:
			return load15(store, act.Head)

		case actorstypes.Version16:
			return load16(store, act.Head)

		case actorstypes.Version17:
			return load17(store, act.Head)

		case actorstypes.Version18:
			return load18(store, act.Head)

		}
	}

	switch act.Code {

	case builtin0.InitActorCodeID:
		return load0(store, act.Head)

	case builtin2.InitActorCodeID:
		return load2(store, act.Head)

	case builtin3.InitActorCodeID:
		return load3(store, act.Head)

	case builtin4.InitActorCodeID:
		return load4(store, act.Head)

	case builtin5.InitActorCodeID:
		return load5(store, act.Head)

	case builtin6.InitActorCodeID:
		return load6(store, act.Head)

	case builtin7.InitActorCodeID:
		return load7(store, act.Head)

	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actorstypes.Version, networkName string) (State, error) {
	switch av {

	case actorstypes.Version0:
		return make0(store, networkName)

	case actorstypes.Version2:
		return make2(store, networkName)

	case actorstypes.Version3:
		return make3(store, networkName)

	case actorstypes.Version4:
		return make4(store, networkName)

	case actorstypes.Version5:
		return make5(store, networkName)

	case actorstypes.Version6:
		return make6(store, networkName)

	case actorstypes.Version7:
		return make7(store, networkName)

	case actorstypes.Version8:
		return make8(store, networkName)

	case actorstypes.Version9:
		return make9(store, networkName)

	case actorstypes.Version10:
		return make10(store, networkName)

	case actorstypes.Version11:
		return make11(store, networkName)

	case actorstypes.Version12:
		return make12(store, networkName)

	case actorstypes.Version13:
		return make13(store, networkName)

	case actorstypes.Version14:
		return make14(store, networkName)

	case actorstypes.Version15:
		return make15(store, networkName)

	case actorstypes.Version16:
		return make16(store, networkName)

	case actorstypes.Version17:
		return make17(store, networkName)

	case actorstypes.Version18:
		return make18(store, networkName)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	cbor.Marshaler

	Code() cid.Cid
	ActorKey() string
	ActorVersion() actorstypes.Version

	ResolveAddress(address address.Address) (address.Address, bool, error)
	MapAddressToNewID(address address.Address) (address.Address, error)
	NetworkName() (dtypes.NetworkName, error)

	ForEachActor(func(id abi.ActorID, address address.Address) error) error

	// Remove exists to support tooling that manipulates state for testing.
	// It should not be used in production code, as init actor entries are
	// immutable.
	Remove(addrs ...address.Address) error

	// Sets the network's name. This should only be used on upgrade/fork.
	SetNetworkName(name string) error

	// Sets the next ID for the init actor. This should only be used for testing.
	SetNextID(id abi.ActorID) error

	// Sets the address map for the init actor. This should only be used for testing.
	SetAddressMap(mcid cid.Cid) error

	GetState() interface{}

	AddressMap() (adt.Map, error)
	AddressMapBitWidth() int
	AddressMapHashFunction() func(input []byte) []byte
}

func AllCodes() []cid.Cid {
	return []cid.Cid{
		(&state0{}).Code(),
		(&state2{}).Code(),
		(&state3{}).Code(),
		(&state4{}).Code(),
		(&state5{}).Code(),
		(&state6{}).Code(),
		(&state7{}).Code(),
		(&state8{}).Code(),
		(&state9{}).Code(),
		(&state10{}).Code(),
		(&state11{}).Code(),
		(&state12{}).Code(),
		(&state13{}).Code(),
		(&state14{}).Code(),
		(&state15{}).Code(),
		(&state16{}).Code(),
		(&state17{}).Code(),
		(&state18{}).Code(),
	}
}
