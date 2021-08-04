package events

import (
	"context"
	"sync"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

type observer struct {
	api EventAPI

	lk           sync.Mutex
	gcConfidence abi.ChainEpoch

	ready chan struct{}

	head      *types.TipSet
	maxHeight abi.ChainEpoch

	observers []TipSetObserver
}

func newObserver(api EventAPI, gcConfidence abi.ChainEpoch) *observer {
	return &observer{
		api:          api,
		gcConfidence: gcConfidence,

		ready:     make(chan struct{}),
		observers: []TipSetObserver{},
	}
}

func (o *observer) start(ctx context.Context) error {
	go o.listenHeadChanges(ctx)

	// Wait for the first tipset to be seen or bail if shutting down
	select {
	case <-o.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (o *observer) listenHeadChanges(ctx context.Context) {
	for {
		if err := o.listenHeadChangesOnce(ctx); err != nil {
			log.Errorf("listen head changes errored: %s", err)
		} else {
			log.Warn("listenHeadChanges quit")
		}
		select {
		case <-build.Clock.After(time.Second):
		case <-ctx.Done():
			log.Warnf("not restarting listenHeadChanges: context error: %s", ctx.Err())
			return
		}

		log.Info("restarting listenHeadChanges")
	}
}

func (o *observer) listenHeadChangesOnce(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	notifs, err := o.api.ChainNotify(ctx)
	if err != nil {
		// Retry is handled by caller
		return xerrors.Errorf("listenHeadChanges ChainNotify call failed: %w", err)
	}

	var cur []*api.HeadChange
	var ok bool

	// Wait for first tipset or bail
	select {
	case cur, ok = <-notifs:
		if !ok {
			return xerrors.Errorf("notification channel closed")
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	if len(cur) != 1 {
		return xerrors.Errorf("unexpected initial head notification length: %d", len(cur))
	}

	if cur[0].Type != store.HCCurrent {
		return xerrors.Errorf("expected first head notification type to be 'current', was '%s'", cur[0].Type)
	}

	head := cur[0].Val
	if o.head == nil {
		o.head = head
		close(o.ready)
	} else if !o.head.Equals(head) {
		changes, err := o.api.ChainGetPath(ctx, o.head.Key(), head.Key())
		if err != nil {
			return xerrors.Errorf("failed to get path from last applied tipset to head: %w", err)
		}

		if err := o.applyChanges(ctx, changes); err != nil {
			return xerrors.Errorf("failed to apply head changes: %w", err)
		}
	}

	for changes := range notifs {
		if err := o.applyChanges(ctx, changes); err != nil {
			return err
		}
	}

	return nil
}

func (o *observer) applyChanges(ctx context.Context, changes []*api.HeadChange) error {
	// Used to wait for a prior notification round to finish (by tests)
	if len(changes) == 0 {
		return nil
	}

	var rev, app []*types.TipSet
	for _, changes := range changes {
		switch changes.Type {
		case store.HCRevert:
			rev = append(rev, changes.Val)
		case store.HCApply:
			app = append(app, changes.Val)
		default:
			log.Errorf("unexpected head change notification type: '%s'", changes.Type)
		}
	}

	if err := o.headChange(ctx, rev, app); err != nil {
		return xerrors.Errorf("failed to apply head changes: %w", err)
	}
	return nil
}

func (o *observer) headChange(ctx context.Context, rev, app []*types.TipSet) error {
	ctx, span := trace.StartSpan(ctx, "events.HeadChange")
	span.AddAttributes(trace.Int64Attribute("reverts", int64(len(rev))))
	span.AddAttributes(trace.Int64Attribute("applies", int64(len(app))))
	defer func() {
		span.AddAttributes(trace.Int64Attribute("endHeight", int64(o.head.Height())))
		span.End()
	}()

	// NOTE: bailing out here if the head isn't what we expected is fine. We'll re-start the
	// entire process and handle any strange reorgs.
	for i, from := range rev {
		if !from.Equals(o.head) {
			return xerrors.Errorf(
				"expected to revert %s (%d), reverting %s (%d)",
				o.head.Key(), o.head.Height(), from.Key(), from.Height(),
			)
		}
		var to *types.TipSet
		if i+1 < len(rev) {
			// If we have more reverts, the next revert is the next head.
			to = rev[i+1]
		} else {
			// At the end of the revert sequenece, we need to looup the joint tipset
			// between the revert sequence and the apply sequence.
			var err error
			to, err = o.api.ChainGetTipSet(ctx, from.Parents())
			if err != nil {
				// Well, this sucks. We'll bail and restart.
				return xerrors.Errorf("failed to get tipset when reverting due to a SetHeead: %w", err)
			}
		}

		// Get the observers late in case an observer registers/unregisters itself.
		o.lk.Lock()
		observers := o.observers
		o.lk.Unlock()

		for _, obs := range observers {
			if err := obs.Revert(ctx, from, to); err != nil {
				log.Errorf("observer %T failed to apply tipset %s (%d) with: %s", obs, from.Key(), from.Height(), err)
			}
		}

		if to.Height() < o.maxHeight-o.gcConfidence {
			log.Errorf("reverted past finality, from %d to %d", o.maxHeight, to.Height())
		}

		o.head = to
	}

	for _, to := range app {
		if to.Parents() != o.head.Key() {
			return xerrors.Errorf(
				"cannot apply %s (%d) with parents %s on top of %s (%d)",
				to.Key(), to.Height(), to.Parents(), o.head.Key(), o.head.Height(),
			)
		}

		// Get the observers late in case an observer registers/unregisters itself.
		o.lk.Lock()
		observers := o.observers
		o.lk.Unlock()

		for _, obs := range observers {
			if err := obs.Apply(ctx, o.head, to); err != nil {
				log.Errorf("observer %T failed to revert tipset %s (%d) with: %s", obs, to.Key(), to.Height(), err)
			}
		}
		o.head = to
		if to.Height() > o.maxHeight {
			o.maxHeight = to.Height()
		}

	}
	return nil
}

// TODO: add a confidence level so we can have observers with difference levels of confidence
func (o *observer) Observe(obs TipSetObserver) {
	o.lk.Lock()
	defer o.lk.Unlock()
	o.observers = append(o.observers, obs)
}
