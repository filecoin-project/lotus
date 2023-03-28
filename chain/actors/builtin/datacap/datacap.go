package datacap

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin10 "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
)

var (
	Address = builtin10.DatacapActorAddr
	Methods = builtin10.MethodsDatacap
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != manifest.DatacapKey {
			return nil, xerrors.Errorf("actor code is not datacap: %s", name)
		}

		switch av {

		case actorstypes.Version9:
			return load9(store, act.Head)

		case actorstypes.Version10:
			return load10(store, act.Head)

		}
	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actorstypes.Version, governor address.Address, bitwidth uint64) (State, error) {
	switch av {

	case actorstypes.Version9:
		return make9(store, governor, bitwidth)

	case actorstypes.Version10:
		return make10(store, governor, bitwidth)

	default:
		return nil, xerrors.Errorf("datacap actor only valid for actors v9 and above, got %d", av)
	}
}

type State interface {
	cbor.Marshaler

	Code() cid.Cid
	ActorKey() string
	ActorVersion() actorstypes.Version

	ForEachClient(func(addr address.Address, dcap abi.StoragePower) error) error
	VerifiedClientDataCap(address.Address) (bool, abi.StoragePower, error)
	Governor() (address.Address, error)
	GetState() interface{}
}

func AllCodes() []cid.Cid {
	return []cid.Cid{
		(&state9{}).Code(),
		(&state10{}).Code(),
	}
}
