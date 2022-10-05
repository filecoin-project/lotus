package system

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	system5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/system"

	"github.com/filecoin-project/lotus/chain/actors/adt"
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

func make5(store adt.Store) (State, error) {
	out := state5{store: store}
	out.State = system5.State{}
	return &out, nil
}

type state5 struct {
	system5.State
	store adt.Store
}

func (s *state5) GetState() interface{} {
	return &s.State
}

func (s *state5) GetBuiltinActors() cid.Cid {

	return cid.Undef

}

func (s *state5) SetBuiltinActors(c cid.Cid) error {

	return xerrors.New("cannot set manifest cid before v8")

}
