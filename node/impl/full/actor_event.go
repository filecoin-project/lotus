package full

import (
	"context"
	"fmt"
	"github.com/ipfs/go-cid"

	"go.uber.org/fx"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/types"
)

type ActorEventAPI interface {
	GetActorEvents(ctx context.Context, filter *types.ActorEventFilter) ([]*types.ActorEvent, error)
	// TODO: Add a subscription API here
}

var (
	_ ActorEventAPI = *new(api.FullNode)
	_ ActorEventAPI = *new(api.Gateway)
)

type ActorEvent struct {
	EventFilterManager   *filter.EventFilterManager
	MaxFilterHeightRange abi.ChainEpoch
}

var _ ActorEventAPI = (*ActorEvent)(nil)

type ActorEventsAPI struct {
	fx.In
	ActorEventAPI
}

func (a *ActorEvent) GetActorEvents(ctx context.Context, filter *types.ActorEventFilter) ([]*types.ActorEvent, error) {
	if a.EventFilterManager == nil {
		return nil, api.ErrNotSupported
	}

	// Create a temporary filter
	f, err := a.EventFilterManager.Install(ctx, filter.FromBlock, filter.ToBlock, cid.Undef, filter.Addresses, filter.Fields, false)
	if err != nil {
		return nil, err
	}
	ces := f.TakeCollectedEvents(ctx)

	_ = a.EventFilterManager.Remove(ctx, f.ID())

	var out []*types.ActorEvent

	for _, e := range ces {
		e := e
		c, err := e.TipSetKey.Cid()
		if err != nil {
			return nil, fmt.Errorf("failed to get tipset cid: %w", err)
		}

		ev := &types.ActorEvent{
			Entries:     e.Entries,
			EmitterAddr: e.EmitterAddr,
			Reverted:    e.Reverted,
			Height:      e.Height,
			TipSetKey:   c,
			MsgCid:      e.MsgCid,
		}

		out = append(out, ev)
	}

	return out, nil
}
