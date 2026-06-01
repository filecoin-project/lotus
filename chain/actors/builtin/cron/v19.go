package cron

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	cron19 "github.com/filecoin-project/go-state-types/builtin/v19/cron"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state19)(nil)

func load19(store adt.Store, root cid.Cid) (State, error) {
	out := state19{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make19(store adt.Store) (State, error) {
	out := state19{store: store}
	out.State = *cron19.ConstructState(cron19.BuiltInEntries())
	return &out, nil
}

type state19 struct {
	cron19.State
	store adt.Store
}

func (s *state19) GetState() interface{} {
	return &s.State
}

func (s *state19) ActorKey() string {
	return manifest.CronKey
}

func (s *state19) ActorVersion() actorstypes.Version {
	return actorstypes.Version19
}

func (s *state19) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
