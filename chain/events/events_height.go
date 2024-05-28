package events

import (
	"context"
	"sync"

	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

type heightHandler struct {
	ts     *types.TipSet
	height abi.ChainEpoch
	called bool

	handle HeightHandler
	revert RevertHandler
}

type heightEvents struct {
	api          EventHelperAPI
	gcConfidence abi.ChainEpoch

	lk                        sync.Mutex
	head                      *types.TipSet
	tsHeights, triggerHeights map[abi.ChainEpoch][]*heightHandler
	lastGc                    abi.ChainEpoch //nolint:structcheck
}

func newHeightEvents(api EventHelperAPI, obs *observer, gcConfidence abi.ChainEpoch) *heightEvents {
	he := &heightEvents{
		api:            api,
		gcConfidence:   gcConfidence,
		tsHeights:      map[abi.ChainEpoch][]*heightHandler{},
		triggerHeights: map[abi.ChainEpoch][]*heightHandler{},
	}
	he.lk.Lock()
	he.head = obs.Observe((*heightEventsObserver)(he))
	he.lk.Unlock()
	return he
}

// ChainAt invokes the specified `HeightHandler` when the chain reaches the
// specified height+confidence threshold. If the chain is rolled-back under the
// specified height, `RevertHandler` will be called.
//
// ts passed to handlers is the tipset at the specified epoch, or above if lower tipsets were null.
//
// The context governs cancellations of this call, it won't cancel the event handler.
func (e *heightEvents) ChainAt(ctx context.Context, hnd HeightHandler, rev RevertHandler, confidence int, h abi.ChainEpoch) error {
	if abi.ChainEpoch(confidence) > e.gcConfidence {
		// Need this to be able to GC effectively.
		return xerrors.Errorf("confidence cannot be greater than gcConfidence: %d > %d", confidence, e.gcConfidence)
	}
	handler := &heightHandler{
		height: h,
		handle: hnd,
		revert: rev,
	}
	triggerAt := h + abi.ChainEpoch(confidence)

	// Here we try to jump onto a moving train. To avoid stopping the train, we release the lock
	// while calling the API and/or the trigger functions. Unfortunately, it's entirely possible
	// (although unlikely) to go back and forth across the trigger heights, so we need to keep
	// going back and forth here till we're synced.
	//
	// TODO: Consider using a worker goroutine so we can just drop the handler in a channel? The
	// downside is that we'd either need a tipset cache, or we'd need to potentially fetch
	// tipsets in-line inside the event loop.
	e.lk.Lock()
	for {
		head := e.head
		if head.Height() >= h {
			// Head is past the handler height. We at least need to stash the tipset to
			// avoid doing this from the main event loop.
			e.lk.Unlock()

			var ts *types.TipSet
			if head.Height() == h {
				ts = head
			} else {
				var err error
				ts, err = e.api.ChainGetTipSetAfterHeight(ctx, handler.height, head.Key())
				if err != nil {
					return xerrors.Errorf("events.ChainAt: failed to get tipset: %s", err)
				}
			}

			// If we've applied the handler on the wrong tipset, revert.
			if handler.called && !ts.Equals(handler.ts) {
				ctx, span := trace.StartSpan(ctx, "events.HeightRevert")
				span.AddAttributes(trace.BoolAttribute("immediate", true))
				err := handler.revert(ctx, handler.ts)
				span.End()
				if err != nil {
					return err
				}
				handler.called = false
			}

			// Save the tipset.
			handler.ts = ts

			// If we've reached confidence and haven't called, call.
			if !handler.called && head.Height() >= triggerAt {
				ctx, span := trace.StartSpan(ctx, "events.HeightApply")
				span.AddAttributes(trace.BoolAttribute("immediate", true))
				err := handler.handle(ctx, handler.ts, head.Height())
				span.End()
				if err != nil {
					return err
				}

				handler.called = true

				// If we've reached gcConfidence, return without saving anything.
				if head.Height() >= h+e.gcConfidence {
					return nil
				}
			}

			e.lk.Lock()
		} else if handler.called {
			// We're not passed the head (anymore) but have applied the handler. Revert, try again.
			e.lk.Unlock()
			ctx, span := trace.StartSpan(ctx, "events.HeightRevert")
			span.AddAttributes(trace.BoolAttribute("immediate", true))
			err := handler.revert(ctx, handler.ts)
			span.End()
			if err != nil {
				return err
			}
			handler.called = false
			e.lk.Lock()
		} // otherwise, we changed heads but the change didn't matter.

		// If we managed to get through this without the head changing, we're finally done.
		if head.Equals(e.head) {
			e.triggerHeights[triggerAt] = append(e.triggerHeights[triggerAt], handler)
			e.tsHeights[h] = append(e.tsHeights[h], handler)
			e.lk.Unlock()
			return nil
		}
	}
}

// Updates the head and garbage collects if we're 2x over our garbage collection confidence period.
func (e *heightEventsObserver) updateHead(h *types.TipSet) {
	e.lk.Lock()
	defer e.lk.Unlock()
	e.head = h

	if e.head.Height() < e.lastGc+e.gcConfidence*2 {
		return
	}
	e.lastGc = h.Height()

	targetGcHeight := e.head.Height() - e.gcConfidence
	for h := range e.tsHeights {
		if h >= targetGcHeight {
			continue
		}
		delete(e.tsHeights, h)
	}
	for h := range e.triggerHeights {
		if h >= targetGcHeight {
			continue
		}
		delete(e.triggerHeights, h)
	}
}

type heightEventsObserver heightEvents

func (e *heightEventsObserver) Revert(ctx context.Context, from, to *types.TipSet) error {
	// Update the head first so we don't accidental skip reverting a concurrent call to ChainAt.
	e.updateHead(to)

	// Call revert on all heights between the two tipsets, handling empty tipsets.
	for h := from.Height(); h > to.Height(); h-- {
		e.lk.Lock()
		triggers := e.tsHeights[h]
		e.lk.Unlock()

		// 1. Triggers are only invoked from the global event loop, we don't need to hold the lock while calling.
		// 2. We only ever append to or replace the trigger slice, so it's safe to iterate over it without the lock.
		for _, handler := range triggers {
			handler.ts = nil // invalidate
			if !handler.called {
				// We haven't triggered this yet, or there has been a concurrent call to ChainAt.
				continue
			}
			ctx, span := trace.StartSpan(ctx, "events.HeightRevert")
			err := handler.revert(ctx, from)
			span.End()

			if err != nil {
				log.Errorf("reverting chain trigger (@H %d): %s", h, err)
			}
			handler.called = false
		}
	}
	return nil
}

func (e *heightEventsObserver) Apply(ctx context.Context, from, to *types.TipSet) error {
	// Update the head first so we don't accidental skip applying a concurrent call to ChainAt.
	e.updateHead(to)

	for h := from.Height() + 1; h <= to.Height(); h++ {
		e.lk.Lock()
		triggers := e.triggerHeights[h]
		tipsets := e.tsHeights[h]
		e.lk.Unlock()

		// Stash the tipset for future triggers.
		for _, handler := range tipsets {
			handler.ts = to
		}

		// Trigger the ready triggers.
		for _, handler := range triggers {
			if handler.called {
				// We may have reverted past the trigger point, but not past the call point.
				// Or there has been a concurrent call to ChainAt.
				continue
			}

			ctx, span := trace.StartSpan(ctx, "events.HeightApply")
			span.AddAttributes(trace.BoolAttribute("immediate", false))
			err := handler.handle(ctx, handler.ts, h)
			span.End()

			if err != nil {
				log.Errorf("chain trigger (@H %d, called @ %d) failed: %+v", h, to.Height(), err)
			}

			handler.called = true
		}
	}
	return nil
}
