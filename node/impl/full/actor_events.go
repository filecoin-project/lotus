package full

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/raulk/clock"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types"
)

type ActorEventAPI interface {
	GetActorEventsRaw(ctx context.Context, filter *types.ActorEventFilter) ([]*types.ActorEvent, error)
	SubscribeActorEventsRaw(ctx context.Context, filter *types.ActorEventFilter) (<-chan *types.ActorEvent, error)
}

var (
	_ ActorEventAPI = *new(api.FullNode)
	_ ActorEventAPI = *new(api.Gateway)
)

type ChainAccessor interface {
	GetHeaviestTipSet() *types.TipSet
}

type EventFilterManager interface {
	Fill(
		ctx context.Context,
		minHeight, maxHeight abi.ChainEpoch,
		tipsetCid cid.Cid,
		addresses []address.Address,
		keysWithCodec map[string][]types.ActorEventBlock,
	) (filter.EventFilter, error)
	Install(
		ctx context.Context,
		minHeight, maxHeight abi.ChainEpoch,
		tipsetCid cid.Cid,
		addresses []address.Address,
		keysWithCodec map[string][]types.ActorEventBlock,
	) (filter.EventFilter, error)
	Remove(ctx context.Context, id types.FilterID) error
}

type ActorEventsAPI struct {
	fx.In
	ActorEventAPI
}

type ActorEventHandler struct {
	chain                ChainAccessor
	eventFilterManager   EventFilterManager
	blockDelay           time.Duration
	maxFilterHeightRange abi.ChainEpoch
	clock                clock.Clock
}

var _ ActorEventAPI = (*ActorEventHandler)(nil)

func NewActorEventHandler(
	chain ChainAccessor,
	eventFilterManager EventFilterManager,
	blockDelay time.Duration,
	maxFilterHeightRange abi.ChainEpoch,
) *ActorEventHandler {
	return &ActorEventHandler{
		chain:                chain,
		eventFilterManager:   eventFilterManager,
		blockDelay:           blockDelay,
		maxFilterHeightRange: maxFilterHeightRange,
		clock:                clock.New(),
	}
}

func NewActorEventHandlerWithClock(
	chain ChainAccessor,
	eventFilterManager EventFilterManager,
	blockDelay time.Duration,
	maxFilterHeightRange abi.ChainEpoch,
	clock clock.Clock,
) *ActorEventHandler {
	return &ActorEventHandler{
		chain:                chain,
		eventFilterManager:   eventFilterManager,
		blockDelay:           blockDelay,
		maxFilterHeightRange: maxFilterHeightRange,
		clock:                clock,
	}
}

func (a *ActorEventHandler) GetActorEventsRaw(ctx context.Context, evtFilter *types.ActorEventFilter) ([]*types.ActorEvent, error) {
	if a.eventFilterManager == nil {
		return nil, api.ErrNotSupported
	}

	if evtFilter == nil {
		evtFilter = &types.ActorEventFilter{}
	}
	params, err := a.parseFilter(*evtFilter)
	if err != nil {
		return nil, err
	}

	// Fill a filter and collect events
	tipSetCid, err := params.GetTipSetCid()
	if err != nil {
		return nil, fmt.Errorf("failed to get tipset cid: %w", err)
	}
	f, err := a.eventFilterManager.Fill(ctx, params.MinHeight, params.MaxHeight, tipSetCid, evtFilter.Addresses, evtFilter.Fields)
	if err != nil {
		return nil, err
	}
	return getCollected(ctx, f), nil
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

		return &filterParams{
			MinHeight: 0,
			MaxHeight: 0,
			TipSetKey: *f.TipSetKey,
		}, nil
	}

	min, max, err := parseHeightRange(a.chain.GetHeaviestTipSet().Height(), f.FromHeight, f.ToHeight, a.maxFilterHeightRange)
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

func (a *ActorEventHandler) SubscribeActorEventsRaw(ctx context.Context, evtFilter *types.ActorEventFilter) (<-chan *types.ActorEvent, error) {
	if a.eventFilterManager == nil {
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
	fm, err := a.eventFilterManager.Install(ctx, params.MinHeight, params.MaxHeight, tipSetCid, evtFilter.Addresses, evtFilter.Fields)
	if err != nil {
		return nil, err
	}

	// The goal for the code below is to send events on the `out` channel as fast as possible and not
	// let it get too far behind the rate at which the events are generated.
	// For historical events, we aim to send all events within a single block's time (30s on mainnet).
	// This ensures that the client can catch up quickly enough to start receiving new events.
	// For ongoing events, we also aim to send all events within a single block's time, so we never
	// want to be buffering events (approximately) more than one epoch behind the current head.
	// It's approximate because we only update our notion of "current epoch" once per ~blocktime.

	out := make(chan *types.ActorEvent)

	// When we start sending real-time events, we want to make sure that we don't fall behind more
	// than one epoch's worth of events (approximately). Capture this value now, before we send
	// historical events to allow for a little bit of slack in the historical event sending.
	minBacklogHeight := a.chain.GetHeaviestTipSet().Height() - 1

	go func() {
		defer func() {
			// tell the caller we're done
			close(out)
			fm.ClearSubChannel()
			if err := a.eventFilterManager.Remove(ctx, fm.ID()); err != nil {
				log.Warnf("failed to remove filter: %s", err)
			}
		}()

		// Handle any historical events that our filter may have picked up -----------------------------

		evs := getCollected(ctx, fm)
		if len(evs) > 0 {
			// ensure we get all events out on the channel within one block's time (30s on mainnet)
			timer := a.clock.Timer(a.blockDelay)
			for _, ev := range evs {
				select {
				case out <- ev:
				case <-timer.C:
					log.Errorf("closing event subscription due to slow event sending rate")
					timer.Stop()
					return
				case <-ctx.Done():
					timer.Stop()
					return
				}
			}
			timer.Stop()
		}

		// for the case where we have a MaxHeight set, we don't get a signal from the filter when we
		// reach that height, so we need to check it ourselves, do it now but also in the loop
		if params.MaxHeight > 0 && minBacklogHeight+1 >= params.MaxHeight {
			return
		}

		// Handle ongoing events from the filter -------------------------------------------------------

		in := make(chan interface{}, 256)
		fm.SetSubChannel(in)

		var buffer []*types.ActorEvent
		nextBacklogHeightUpdate := a.clock.Now().Add(a.blockDelay)

		collectEvent := func(ev interface{}) bool {
			ce, ok := ev.(*index.CollectedEvent)
			if !ok {
				log.Errorf("got unexpected value from event filter: %T", ev)
				return false
			}

			if ce.Height < minBacklogHeight {
				// since we mostly care about buffer size, we only trigger a too-slow close when the buffer
				// increases, i.e. we collect a new event
				log.Errorf("closing event subscription due to slow event sending rate")
				return false
			}

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

		ticker := a.clock.Ticker(a.blockDelay)
		defer ticker.Stop()

		for ctx.Err() == nil {
			if len(buffer) > 0 {
				select {
				case ev, ok := <-in: // incoming event
					if !ok || !collectEvent(ev) {
						return
					}
				case out <- buffer[0]: // successful send
					buffer[0] = nil
					buffer = buffer[1:]
				case <-ticker.C:
					// check that our backlog isn't too big by looking at the oldest event
					if buffer[0].Height < minBacklogHeight {
						log.Errorf("closing event subscription due to slow event sending rate")
						return
					}
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
				case <-ticker.C:
					currentHeight := a.chain.GetHeaviestTipSet().Height()
					if params.MaxHeight > 0 && currentHeight > params.MaxHeight {
						// we've reached the filter's MaxHeight, we're done so we can close the channel
						return
					}
				}
			}

			if a.clock.Now().After(nextBacklogHeightUpdate) {
				minBacklogHeight = a.chain.GetHeaviestTipSet().Height() - 1
				nextBacklogHeightUpdate = a.clock.Now().Add(a.blockDelay)
			}
		}
	}()

	return out, nil
}

func getCollected(ctx context.Context, f filter.EventFilter) []*types.ActorEvent {
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

	return out
}
