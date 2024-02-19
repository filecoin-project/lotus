package full

import (
	"context"
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/store"
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

type ActorEventHandler struct {
	EventFilterManager   *filter.EventFilterManager
	MaxFilterHeightRange abi.ChainEpoch
	Chain                *store.ChainStore
}

var _ ActorEventAPI = (*ActorEventHandler)(nil)

type ActorEventsAPI struct {
	fx.In
	ActorEventAPI
}

func (a *ActorEventHandler) GetActorEvents(ctx context.Context, filter *types.ActorEventFilter) ([]*types.ActorEvent, error) {
	if a.EventFilterManager == nil {
		return nil, api.ErrNotSupported
	}

	params, err := a.parseFilter(filter)
	if err != nil {
		return nil, err
	}

	// Create a temporary filter
	f, err := a.EventFilterManager.Install(ctx, params.MinHeight, params.MaxHeight, params.TipSetCid, filter.Addresses, filter.Fields, false)
	if err != nil {
		return nil, err
	}

	evs, err := getCollected(ctx, f)
	_ = a.EventFilterManager.Remove(ctx, f.ID())
	return evs, err
}

type filterParams struct {
	MinHeight abi.ChainEpoch
	MaxHeight abi.ChainEpoch
	TipSetCid cid.Cid
}

func (a *ActorEventHandler) parseFilter(f *types.ActorEventFilter) (*filterParams, error) {
	if f.TipSetCid != nil {
		if len(f.FromEpoch) != 0 || len(f.ToEpoch) != 0 {
			return nil, fmt.Errorf("cannot specify both TipSetCid and FromEpoch/ToEpoch")
		}

		return &filterParams{
			MinHeight: 0,
			MaxHeight: 0,
			TipSetCid: *f.TipSetCid,
		}, nil
	}

	from := f.FromEpoch
	if len(from) != 0 && from != "latest" && from != "earliest" && !strings.HasPrefix(from, "0x") {
		from = "0x" + from
	}

	to := f.ToEpoch
	if len(to) != 0 && to != "latest" && to != "earliest" && !strings.HasPrefix(to, "0x") {
		to = "0x" + to
	}

	min, max, err := parseBlockRange(a.Chain.GetHeaviestTipSet().Height(), &from, &to, a.MaxFilterHeightRange)
	if err != nil {
		return nil, err
	}

	return &filterParams{
		MinHeight: min,
		MaxHeight: max,
		TipSetCid: cid.Undef,
	}, nil
}

func (a *ActorEventHandler) SubscribeActorEvents(ctx context.Context, f *types.SubActorEventFilter) (<-chan *types.ActorEvent, error) {
	if a.EventFilterManager == nil {
		return nil, api.ErrNotSupported
	}

	params, err := a.parseFilter(&f.Filter)
	if err != nil {
		return nil, err
	}

	fm, err := a.EventFilterManager.Install(ctx, params.MinHeight, params.MaxHeight, params.TipSetCid, f.Filter.Addresses, f.Filter.Fields, false)
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

		if f.Prefill {
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
					TipSetCid:   c,
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
			TipSetCid:   c,
			MsgCid:      e.MsgCid,
		}

		out = append(out, ev)
	}

	return out, nil
}
