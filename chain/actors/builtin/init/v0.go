package init

import (
	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/specs-actors/actors/builtin/init"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

type v0State struct {
	init.State
	store adt.Store
}

func (s *v0State) ResolveAddress(address address.Address) (address.Address, bool, error) {
	return s.State.ResolveAddress(s.store, address)
}

func (s *v0State) MapAddressToNewID(address address.Address) (address.Address, error) {
	return s.State.MapAddressToNewID(s.store, address)
}
