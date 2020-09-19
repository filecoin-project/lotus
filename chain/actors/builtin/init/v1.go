package init

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/node/modules/dtypes"

	init_ "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"
	adt1 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"
)

var _ State = (*state1)(nil)

type state1 struct {
	init_.State
	store adt.Store
}

func (s *state1) ResolveAddress(address address.Address) (address.Address, bool, error) {
	return s.State.ResolveAddress(s.store, address)
}

func (s *state1) MapAddressToNewID(address address.Address) (address.Address, error) {
	return s.State.MapAddressToNewID(s.store, address)
}

func (s *state1) ForEachActor(cb func(id abi.ActorID, address address.Address) error) error {
	addrs, err := adt1.AsMap(s.store, s.State.AddressMap)
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

func (s *state1) NetworkName() (dtypes.NetworkName, error) {
	return dtypes.NetworkName(s.State.NetworkName), nil
}
