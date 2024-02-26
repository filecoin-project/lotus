package full

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
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

	evs, _, _ := getCollected(ctx, f)
	if err := a.EventFilterManager.Remove(ctx, f.ID()); err != nil {
		log.Warnf("failed to remove filter: %s", err)
	}
	return evs, nil
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

	// The goal for the code below is to be able to send events on the `out` channel as fast as
	// possible and not let it get too far behind the rate at which the events are generated.
	// For historical events we see the rate at which they were generated by looking the height range;
	// we then make sure that the client can receive them at least twice as fast as they were
	// generated so they catch up quick enough to receive new events.
	// For ongoing events we use an exponential moving average of the events per height to make sure
	// that the client doesn't fall behind.
	// In both cases we allow a little bit of slack but need to avoid letting the client bloat the
	// buffer too much.
	// There is no special handling for reverts, so they will just look like a lot more events per
	// epoch and the user has to receive them anyway.

	out := make(chan *types.ActorEvent)

	go func() {
		defer func() {
			// tell the caller we're done
			close(out)
			fm.ClearSubChannel()
			if err := a.EventFilterManager.Remove(ctx, fm.ID()); err != nil {
				log.Warnf("failed to remove filter: %s", err)
			}
		}()

		// Handle any historical events that our filter may have picked up -----------------------------

		evs, minEpoch, maxEpoch := getCollected(ctx, fm)
		if len(evs) > 0 {
			// must be able to send events at least twice as fast as they were generated
			epochRange := maxEpoch - minEpoch
			if epochRange <= 0 {
				epochRange = 1
			}
			eventsPerEpoch := float64(len(evs)) / float64(epochRange)
			eventsPerSecond := 2 * eventsPerEpoch / float64(build.BlockDelaySecs)
			// a minimum rate of 1 event per second if we don't have many events
			if eventsPerSecond < 1 {
				eventsPerSecond = 1
			}

			// send events from evs to the out channel and ensure we don't do it slower than eventsPerMs
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			const maxSlowTicks = 3 // slightly forgiving, allow 3 slow ticks (seconds) before giving up
			slowTicks := 0
			sentEvents := 0.0

			for _, ev := range evs {
				select {
				case out <- ev:
					sentEvents++
				case <-ticker.C:
					if sentEvents < eventsPerSecond {
						slowTicks++
						if slowTicks >= maxSlowTicks {
							log.Errorf("closing event subscription due to slow event sending rate")
							return
						}
					} else {
						slowTicks = 0
					}
					sentEvents = 0
				case <-ctx.Done():
					return
				}
			}
		}

		// Handle ongoing events from the filter -------------------------------------------------------

		in := make(chan interface{}, 256)
		fm.SetSubChannel(in)

		var buffer []*types.ActorEvent
		const α = 0.2                        // decay factor for the events per height EMA
		var eventsPerHeightEma float64 = 256 // exponential moving average of events per height, initially guess at 256
		var lastHeight abi.ChainEpoch        // last seen event height
		var eventsAtCurrentHeight int        // number of events at the current height

		collectEvent := func(ev interface{}) bool {
			ce, ok := ev.(*filter.CollectedEvent)
			if !ok {
				log.Errorf("got unexpected value from event filter: %T", ev)
				return false
			}

			if ce.Height > lastHeight {
				// update the EMA of events per height when the height increases
				if lastHeight != 0 {
					eventsPerHeightEma = α*float64(eventsAtCurrentHeight) + (1-α)*eventsPerHeightEma
				}
				lastHeight = ce.Height
				eventsAtCurrentHeight = 0
			}
			eventsAtCurrentHeight++

			buffer = append(buffer, &types.ActorEvent{
				Entries:   ce.Entries,
				Emitter:   ce.EmitterAddr,
				Reverted:  ce.Reverted,
				Height:    ce.Height,
				TipSetKey: ce.TipSetKey,
				MsgCid:    ce.MsgCid,
			})
			return true
		}

		for ctx.Err() == nil {
			if len(buffer) > 0 {
				// check if we need to disconnect the client because they've fallen behind, always allow at
				// least 8 events in the buffer to provide a little bit of slack
				if len(buffer) > 8 && float64(len(buffer)) > eventsPerHeightEma/2 {
					log.Errorf("closing event subscription due to slow event sending rate")
					return
				}

				select {
				case ev, ok := <-in: // incoming event
					if !ok || !collectEvent(ev) {
						return
					}
				case out <- buffer[0]: // successful send
					buffer[0] = nil
					buffer = buffer[1:]
				case <-ctx.Done():
					return
				}
			} else {
				select {
				case ev, ok := <-in: // incoming event
					if !ok || !collectEvent(ev) {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

func getCollected(ctx context.Context, f *filter.EventFilter) ([]*types.ActorEvent, abi.ChainEpoch, abi.ChainEpoch) {
	ces := f.TakeCollectedEvents(ctx)

	var out []*types.ActorEvent
	var min, max abi.ChainEpoch

	for _, e := range ces {
		if min == 0 || e.Height < min {
			min = e.Height
		}
		if e.Height > max {
			max = e.Height
		}
		out = append(out, &types.ActorEvent{
			Entries:   e.Entries,
			Emitter:   e.EmitterAddr,
			Reverted:  e.Reverted,
			Height:    e.Height,
			TipSetKey: e.TipSetKey,
			MsgCid:    e.MsgCid,
		})
	}

	return out, min, max
}
