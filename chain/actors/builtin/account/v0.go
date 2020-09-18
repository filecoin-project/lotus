package account

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
)

type v0State struct {
	account.State
	store adt.Store
}

func (s *v0State) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}
