package account

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
)

type state0 struct {
	account.State
	store adt.Store
}

func (s *state0) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}
