package full

import (
	"context"
	"errors"

	"github.com/filecoin-project/lotus/chain/types"
)

var ErrActorEventModuleDisabled = errors.New("module disabled, enable with Events.EnableActorEventsAPI")

type ActorEventDummy struct{}

func (a *ActorEventDummy) GetActorEventsRaw(ctx context.Context, filter *types.ActorEventFilter) ([]*types.ActorEvent, error) {
	return nil, ErrActorEventModuleDisabled
}

func (a *ActorEventDummy) SubscribeActorEventsRaw(ctx context.Context, filter *types.ActorEventFilter) (<-chan *types.ActorEvent, error) {
	return nil, ErrActorEventModuleDisabled
}

var _ ActorEventAPI = &ActorEventDummy{}
