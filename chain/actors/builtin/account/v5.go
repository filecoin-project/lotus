package account

import (
	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	account5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/account"
)

var _ State = (*state5)(nil)

func load5(store adt.Store, root cid.Cid) (State, error) {
	out := state5{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type state5 struct {
	account5.State
	store adt.Store
}

func (s *state5) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}
