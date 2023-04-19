package evm

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin11 "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
)

var Methods = builtin11.MethodsEVM

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != manifest.EvmKey {
			return nil, xerrors.Errorf("actor code is not evm: %s", name)
		}

		switch av {

		case actorstypes.Version10:
			return load10(store, act.Head)

		case actorstypes.Version11:
			return load11(store, act.Head)

		}
	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actorstypes.Version, bytecode cid.Cid) (State, error) {
	switch av {

	case actorstypes.Version10:
		return make10(store, bytecode)

	case actorstypes.Version11:
		return make11(store, bytecode)

	default:
		return nil, xerrors.Errorf("evm actor only valid for actors v10 and above, got %d", av)
	}
}

type State interface {
	cbor.Marshaler

	Nonce() (uint64, error)
	IsAlive() (bool, error)
	GetState() interface{}

	GetBytecode() ([]byte, error)
	GetBytecodeCID() (cid.Cid, error)
	GetBytecodeHash() ([32]byte, error)
}
