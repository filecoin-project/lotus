package events

import (
	"context"
	"math"
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

const NoTimeout = math.MaxUint64

type triggerId = uint64

// msgH is the block height at which a message was present / event has happened
type msgH = uint64

// triggerH is the block height at which the listener will be notified about the
//  message (msgH+confidence)
type triggerH = uint64

// `ts` is the tipset, in which the `msg` is included.
// `curH`-`ts.Height` = `confidence`
type CalledHandler func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH uint64) (more bool, err error)

// CheckFunc is used for atomicity guarantees. If the condition the callbacks
// wait for has already happened in tipset `ts`
//
// If `done` is true, timeout won't be triggered
// If `more` is false, no messages will be sent to CalledHandler (RevertHandler
//  may still be called)
type CheckFunc func(ts *types.TipSet) (done bool, more bool, err error)

type MatchFunc func(msg *types.Message) (bool, error)

type callHandler struct {
	confidence int
	timeout    uint64

	disabled bool // TODO: GC after gcConfidence reached

	handle CalledHandler
	revert RevertHandler
}

type queuedEvent struct {
	trigger triggerId

	h   uint64
	msg *types.Message

	called bool
}

type calledEvents struct {
	cs           eventApi
	tsc          *tipSetCache
	ctx          context.Context
	gcConfidence uint64

	lk sync.Mutex

	ctr triggerId

	triggers map[triggerId]*callHandler
	matchers map[triggerId][]MatchFunc

	// maps block heights to events
	// [triggerH][msgH][event]
	confQueue map[triggerH]map[msgH][]*queuedEvent

	// [msgH][triggerH]
	revertQueue map[msgH][]triggerH

	// [timeoutH+confidence][triggerId]{calls}
	timeouts map[uint64]map[triggerId]int
}

func (e *calledEvents) headChangeCalled(rev, app []*types.TipSet) error {
	for _, ts := range rev {
		e.handleReverts(ts)
	}

	for _, ts := range app {
		// called triggers

		e.checkNewCalls(ts)
		e.applyWithConfidence(ts)
		e.applyTimeouts(ts)
	}

	return nil
}

func (e *calledEvents) handleReverts(ts *types.TipSet) {
	reverts, ok := e.revertQueue[ts.Height()]
	if !ok {
		return // nothing to do
	}

	for _, triggerH := range reverts {
		toRevert := e.confQueue[triggerH][ts.Height()]
		for _, event := range toRevert {
			if !event.called {
				continue // event wasn't apply()-ied yet
			}

			trigger := e.triggers[event.trigger]

			if err := trigger.revert(e.ctx, ts); err != nil {
				log.Errorf("reverting chain trigger (call %s.%d() @H %d, called @ %d) failed: %s", event.msg.To, event.msg.Method, ts.Height(), triggerH, err)
			}
		}
		delete(e.confQueue[triggerH], ts.Height())
	}
	delete(e.revertQueue, ts.Height())
}

func (e *calledEvents) checkNewCalls(ts *types.TipSet) {
	e.messagesForTs(ts, func(msg *types.Message) {
		// TODO: provide receipts

		for tid, matchFns := range e.matchers {
			var matched bool
			for _, matchFn := range matchFns {
				ok, err := matchFn(msg)
				if err != nil {
					log.Warnf("event matcher failed: %s")
					continue
				}
				matched = ok

				if matched {
					break
				}
			}

			if matched {
				e.queueForConfidence(tid, msg, ts)
				break
			}
		}
	})
}

func (e *calledEvents) queueForConfidence(triggerId uint64, msg *types.Message, ts *types.TipSet) {
	trigger := e.triggers[triggerId]

	// messages are not applied in the tipset they are included in
	appliedH := ts.Height() + 1

	triggerH := appliedH + uint64(trigger.confidence)

	byOrigH, ok := e.confQueue[triggerH]
	if !ok {
		byOrigH = map[uint64][]*queuedEvent{}
		e.confQueue[triggerH] = byOrigH
	}

	byOrigH[appliedH] = append(byOrigH[appliedH], &queuedEvent{
		trigger: triggerId,
		h:       appliedH,
		msg:     msg,
	})

	e.revertQueue[appliedH] = append(e.revertQueue[appliedH], triggerH)
}

func (e *calledEvents) applyWithConfidence(ts *types.TipSet) {
	byOrigH, ok := e.confQueue[ts.Height()]
	if !ok {
		return // no triggers at thin height
	}

	for origH, events := range byOrigH {
		triggerTs, err := e.tsc.get(origH)
		if err != nil {
			log.Errorf("events: applyWithConfidence didn't find tipset for event; wanted %d; current %d", origH, ts.Height())
		}

		for _, event := range events {
			if event.called {
				continue
			}

			trigger := e.triggers[event.trigger]
			if trigger.disabled {
				continue
			}

			rec, err := e.cs.StateGetReceipt(e.ctx, event.msg.Cid(), ts.Key())
			if err != nil {
				log.Error(err)
				return
			}

			more, err := trigger.handle(event.msg, rec, triggerTs, ts.Height())
			if err != nil {
				log.Errorf("chain trigger (call %s.%d() @H %d, called @ %d) failed: %s", event.msg.To, event.msg.Method, origH, ts.Height(), err)
				continue // don't revert failed calls
			}

			event.called = true

			touts, ok := e.timeouts[trigger.timeout]
			if ok {
				touts[event.trigger]++
			}

			trigger.disabled = !more
		}
	}
}

func (e *calledEvents) applyTimeouts(ts *types.TipSet) {
	triggers, ok := e.timeouts[ts.Height()]
	if !ok {
		return // nothing to do
	}

	for triggerId, calls := range triggers {
		if calls > 0 {
			continue // don't timeout if the method was called
		}
		trigger := e.triggers[triggerId]
		if trigger.disabled {
			continue
		}

		timeoutTs, err := e.tsc.get(ts.Height() - uint64(trigger.confidence))
		if err != nil {
			log.Errorf("events: applyTimeouts didn't find tipset for event; wanted %d; current %d", ts.Height()-uint64(trigger.confidence), ts.Height())
		}

		more, err := trigger.handle(nil, nil, timeoutTs, ts.Height())
		if err != nil {
			log.Errorf("chain trigger (call @H %d, called @ %d) failed: %s", timeoutTs.Height(), ts.Height(), err)
			continue // don't revert failed calls
		}

		trigger.disabled = !more // allows messages after timeout
	}
}

func (e *calledEvents) messagesForTs(ts *types.TipSet, consume func(*types.Message)) {
	seen := map[cid.Cid]struct{}{}

	for _, tsb := range ts.Blocks() {

		msgs, err := e.cs.ChainGetBlockMessages(context.TODO(), tsb.Cid())
		if err != nil {
			log.Errorf("messagesForTs MessagesForBlock failed (ts.H=%d, Bcid:%s, B.Mcid:%s): %s", ts.Height(), tsb.Cid(), tsb.Messages, err)
			// this is quite bad, but probably better than missing all the other updates
			continue
		}

		for _, m := range msgs.BlsMessages {
			_, ok := seen[m.Cid()]
			if ok {
				continue
			}
			seen[m.Cid()] = struct{}{}

			consume(m)
		}

		for _, m := range msgs.SecpkMessages {
			_, ok := seen[m.Message.Cid()]
			if ok {
				continue
			}
			seen[m.Message.Cid()] = struct{}{}

			consume(&m.Message)
		}
	}
}

// Called registers a callbacks which are triggered when a specified method is
//  called on an actor, or a timeout is reached.
//
// * `CheckFunc` callback is invoked immediately with a recent tipset, it
//    returns two booleans - `done`, and `more`.
//
//  * `done` should be true when some on-chain action we are waiting for has
//    happened. When `done` is set to true, timeout trigger is disabled.
//
//  * `more` should be false when we don't want to receive new notifications
//    through CalledHandler. Note that notifications may still be delivered to
//    RevertHandler
//
// * `CalledHandler` is called when the specified event was observed on-chain,
//    and a confidence threshold was reached, or the specified `timeout` height
//    was reached with no events observed. When this callback is invoked on a
//    timeout, `msg` is set to nil. This callback returns a boolean specifying
//    whether further notifications should be sent, like `more` return param
//    from `CheckFunc` above.
//
// * `RevertHandler` is called after apply handler, when we drop the tipset
//    containing the message. The tipset passed as the argument is the tipset
//    that is being dropped. Note that the message dropped may be re-applied
//    in a different tipset in small amount of time.
func (e *calledEvents) Called(check CheckFunc, hnd CalledHandler, rev RevertHandler, confidence int, timeout uint64, mf MatchFunc) error {
	e.lk.Lock()
	defer e.lk.Unlock()

	ts := e.tsc.best()
	done, more, err := check(ts)
	if err != nil {
		return xerrors.Errorf("called check error (h: %d): %w", ts.Height(), err)
	}
	if done {
		timeout = NoTimeout
	}

	id := e.ctr
	e.ctr++

	e.triggers[id] = &callHandler{
		confidence: confidence,
		timeout:    timeout + uint64(confidence),

		disabled: !more,

		handle: hnd,
		revert: rev,
	}

	e.matchers[id] = append(e.matchers[id], mf)

	if timeout != NoTimeout {
		if e.timeouts[timeout+uint64(confidence)] == nil {
			e.timeouts[timeout+uint64(confidence)] = map[uint64]int{}
		}
		e.timeouts[timeout+uint64(confidence)][id] = 0
	}

	return nil
}

func (e *calledEvents) CalledMsg(ctx context.Context, hnd CalledHandler, rev RevertHandler, confidence int, timeout uint64, msg store.ChainMsg) error {
	return e.Called(e.CheckMsg(ctx, msg, hnd), hnd, rev, confidence, timeout, e.MatchMsg(msg.VMMessage()))
}
