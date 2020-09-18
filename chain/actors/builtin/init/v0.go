package init

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	cbg "github.com/whyrusleeping/cbor-gen"

	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/node/modules/dtypes"

	v0adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type v0State struct {
	init_.State
	store adt.Store
}

func (s *v0State) ResolveAddress(address address.Address) (address.Address, bool, error) {
	return s.State.ResolveAddress(s.store, address)
}

func (s *v0State) MapAddressToNewID(address address.Address) (address.Address, error) {
	return s.State.MapAddressToNewID(s.store, address)
}

func (s *v0State) ForEachActor(cb func(id abi.ActorID, address address.Address) error) error {
	addrs, err := v0adt.AsMap(s.store, s.State.AddressMap)
	if err != nil {
		return err
	}
	var actorID cbg.CborInt
	return addrs.ForEach(&actorID, func(key string) error {
		addr, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(abi.ActorID(actorID), addr)
	})
}

func (s *v0State) NetworkName() (dtypes.NetworkName, error) {
	return dtypes.NetworkName(s.State.NetworkName), nil
}
