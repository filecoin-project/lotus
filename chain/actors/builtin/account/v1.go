package account

import (
	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	account1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/account"
)

var _ State = (*state1)(nil)

func load1(store adt.Store, root cid.Cid) (State, error) {
	out := state1{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type state1 struct {
	account1.State
	store adt.Store
}

func (s *state1) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}
