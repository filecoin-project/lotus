package full

import (
	"context"
	"fmt"

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
	SubscribeActorEvents(ctx context.Context, filter *types.ActorEventFilter) (<-chan *types.ActorEvent, error)
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

func (a *ActorEventHandler) GetActorEvents(ctx context.Context, evtFilter *types.ActorEventFilter) ([]*types.ActorEvent, error) {
	if a.EventFilterManager == nil {
		return nil, api.ErrNotSupported
	}

	if evtFilter == nil {
		evtFilter = &types.ActorEventFilter{}
	}
	params, err := a.parseFilter(*evtFilter)
	if err != nil {
		return nil, err
	}

	// Install a filter just for this call, collect events, remove the filter

	tipSetCid, err := params.GetTipSetCid()
	if err != nil {
		return nil, fmt.Errorf("failed to get tipset cid: %w", err)
	}
	f, err := a.EventFilterManager.Install(ctx, params.MinHeight, params.MaxHeight, tipSetCid, evtFilter.Addresses, evtFilter.Fields, false)
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
	TipSetKey types.TipSetKey
}

func (fp filterParams) GetTipSetCid() (cid.Cid, error) {
	if fp.TipSetKey.IsEmpty() {
		return cid.Undef, nil
	}
	return fp.TipSetKey.Cid()
}

func (a *ActorEventHandler) parseFilter(f types.ActorEventFilter) (*filterParams, error) {
	if f.TipSetKey != nil && !f.TipSetKey.IsEmpty() {
		if f.FromHeight != nil || f.ToHeight != nil {
			return nil, fmt.Errorf("cannot specify both TipSetKey and FromHeight/ToHeight")
		}

		tsk := types.EmptyTSK
		if f.TipSetKey != nil {
			tsk = *f.TipSetKey
		}
		return &filterParams{
			MinHeight: 0,
			MaxHeight: 0,
			TipSetKey: tsk,
		}, nil
	}

	min, max, err := parseHeightRange(a.Chain.GetHeaviestTipSet().Height(), f.FromHeight, f.ToHeight, a.MaxFilterHeightRange)
	if err != nil {
		return nil, err
	}

	return &filterParams{
		MinHeight: min,
		MaxHeight: max,
		TipSetKey: types.EmptyTSK,
	}, nil
}

// parseHeightRange is similar to eth's parseBlockRange but with slightly different semantics but
// results in equivalent values that we can plug in to the EventFilterManager.
//
// * Uses "height", allowing for nillable values rather than strings
// * No "latest" and "earliest", those are now represented by nil on the way in and -1 on the way out
// * No option for hex representation
func parseHeightRange(heaviest abi.ChainEpoch, fromHeight, toHeight *abi.ChainEpoch, maxRange abi.ChainEpoch) (minHeight abi.ChainEpoch, maxHeight abi.ChainEpoch, err error) {
	if fromHeight != nil && *fromHeight < 0 {
		return 0, 0, fmt.Errorf("range 'from' must be greater than or equal to 0")
	}
	if fromHeight == nil {
		minHeight = -1
	} else {
		minHeight = *fromHeight
	}
	if toHeight == nil {
		maxHeight = -1
	} else {
		maxHeight = *toHeight
	}

	// Validate height ranges are within limits set by node operator
	if minHeight == -1 && maxHeight > 0 {
		// Here the client is looking for events between the head and some future height
		if maxHeight-heaviest > maxRange {
			return 0, 0, fmt.Errorf("invalid epoch range: 'to' height is too far in the future (maximum: %d)", maxRange)
		}
	} else if minHeight >= 0 && maxHeight == -1 {
		// Here the client is looking for events between some time in the past and the current head
		if heaviest-minHeight > maxRange {
			return 0, 0, fmt.Errorf("invalid epoch range: 'from' height is too far in the past (maximum: %d)", maxRange)
		}
	} else if minHeight >= 0 && maxHeight >= 0 {
		if minHeight > maxHeight {
			return 0, 0, fmt.Errorf("invalid epoch range: 'to' height (%d) must be after 'from' height (%d)", minHeight, maxHeight)
		} else if maxHeight-minHeight > maxRange {
			return 0, 0, fmt.Errorf("invalid epoch range: range between to and 'from' heights is too large (maximum: %d)", maxRange)
		}
	}
	return minHeight, maxHeight, nil
}

func (a *ActorEventHandler) SubscribeActorEvents(ctx context.Context, evtFilter *types.ActorEventFilter) (<-chan *types.ActorEvent, error) {
	if a.EventFilterManager == nil {
		return nil, api.ErrNotSupported
	}
	if evtFilter == nil {
		evtFilter = &types.ActorEventFilter{}
	}
	params, err := a.parseFilter(*evtFilter)
	if err != nil {
		return nil, err
	}

	tipSetCid, err := params.GetTipSetCid()
	if err != nil {
		return nil, fmt.Errorf("failed to get tipset cid: %w", err)
	}
	fm, err := a.EventFilterManager.Install(ctx, params.MinHeight, params.MaxHeight, tipSetCid, evtFilter.Addresses, evtFilter.Fields, false)
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
				// TODO: need to fix this, buffer of 25 isn't going to work for prefill without a _really_ fast client or a small number of events
				log.Errorf("closing event subscription due to slow reader")
				return
			}
		}

		in := make(chan interface{}, 256)
		fm.SetSubChannel(in)

		for ctx.Err() == nil {
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

				ev := &types.ActorEvent{
					Entries:   ce.Entries,
					Emitter:   ce.EmitterAddr,
					Reverted:  ce.Reverted,
					Height:    ce.Height,
					TipSetKey: ce.TipSetKey,
					MsgCid:    ce.MsgCid,
				}

				select {
				case out <- ev:
				default: // TODO: need to fix this to be more intelligent about the consumption rate vs the accumulation rate
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
		out = append(out, &types.ActorEvent{
			Entries:   e.Entries,
			Emitter:   e.EmitterAddr,
			Reverted:  e.Reverted,
			Height:    e.Height,
			TipSetKey: e.TipSetKey,
			MsgCid:    e.MsgCid,
		})
	}

	return out, nil
}
