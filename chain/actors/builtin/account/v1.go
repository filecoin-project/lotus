package account

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin/account"
)

var _ State = (*state1)(nil)

type state1 struct {
	account.State
	store adt.Store
}

func (s *state1) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}
