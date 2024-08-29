package system

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/manifest"
	system2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/system"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state2)(nil)

func load2(store adt.Store, root cid.Cid) (State, error) {
	out := state2{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make2(store adt.Store) (State, error) {
	out := state2{store: store}
	out.State = system2.State{}
	return &out, nil
}

type state2 struct {
	system2.State
	store adt.Store
}

func (s *state2) GetState() interface{} {
	return &s.State
}

func (s *state2) GetBuiltinActors() cid.Cid {

	return cid.Undef

}

func (s *state2) SetBuiltinActors(c cid.Cid) error {

	return xerrors.New("cannot set manifest cid before v8")

}

func (s *state2) ActorKey() string {
	return manifest.SystemKey
}

func (s *state2) ActorVersion() actorstypes.Version {
	return actorstypes.Version2
}

func (s *state2) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
