package init

import (
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/cbor"
	"golang.org/x/xerrors"

	v0builtin "github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	switch act.Code {
	case v0builtin.InitActorCodeID:
		out := v0State{store: store}
		err := store.Get(store.Context(), act.Head, &out)
		if err != nil {
			return nil, err
		}
		return &out, nil
	}
	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

type State interface {
	cbor.Marshaler

	ResolveAddress(address addr.Address) (address.Address, bool, error)
	MapAddressToNewID(address addr.Address) (address.Address, error)
}
