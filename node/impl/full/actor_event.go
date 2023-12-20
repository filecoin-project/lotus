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
	SubscribeActorEvents(ctx context.Context, filter *types.SubActorEventFilter) (<-chan *types.ActorEvent, error)
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
	f, err := a.EventFilterManager.Install(ctx, filter.MinEpoch, filter.MaxEpoch, cid.Undef, filter.Addresses, filter.Fields, false)
	if err != nil {
		return nil, err
	}

	evs, err := getCollected(ctx, f)
	_ = a.EventFilterManager.Remove(ctx, f.ID())
	return evs, err
}

func (a *ActorEvent) SubscribeActorEvents(ctx context.Context, f *types.SubActorEventFilter) (<-chan *types.ActorEvent, error) {
	if a.EventFilterManager == nil {
		return nil, api.ErrNotSupported
	}
	fm, err := a.EventFilterManager.Install(ctx, f.MinEpoch, f.MaxEpoch, cid.Undef, f.Addresses, f.Fields, false)
	if err != nil {
		return nil, err
	}

	out := make(chan *types.ActorEvent, 25)

	go func() {
		defer func() {
			// Tell the caller we're done
			close(out)

			// Unsubscribe.
			fm.ClearSubChannel()
			_ = a.EventFilterManager.Remove(ctx, fm.ID())
		}()

		if f.WriteExisting {
			evs, err := getCollected(ctx, fm)
			if err != nil {
				log.Errorf("failed to get collected events: %w", err)
				return
			}

			for _, ev := range evs {
				ev := ev
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				default:
					log.Errorf("closing event subscription due to slow reader")
					return
				}
			}
		}

		in := make(chan interface{}, 256)
		fm.SetSubChannel(in)

		for {
			select {
			case val, ok := <-in:
				if !ok {
					// Shutting down.
					return
				}

				ce, ok := val.(*filter.CollectedEvent)
				if !ok {
					log.Errorf("got unexpected value from event filter: %T", val)
					return
				}
				c, err := ce.TipSetKey.Cid()
				if err != nil {
					log.Errorf("failed to get tipset cid: %w", err)
					return
				}

				ev := &types.ActorEvent{
					Entries:     ce.Entries,
					EmitterAddr: ce.EmitterAddr,
					Reverted:    ce.Reverted,
					Height:      ce.Height,
					TipSetKey:   c,
					MsgCid:      ce.MsgCid,
				}

				select {
				case out <- ev:
				default:
					log.Errorf("closing event subscription due to slow reader")
					return
				}
				if len(out) > 5 {
					log.Warnf("event subscription is slow, has %d buffered entries", len(out))
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func getCollected(ctx context.Context, f *filter.EventFilter) ([]*types.ActorEvent, error) {
	ces := f.TakeCollectedEvents(ctx)

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
