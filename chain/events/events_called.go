package events

import (
	"context"
	"math"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
)

const NoTimeout = math.MaxInt64
const NoHeight = abi.ChainEpoch(-1)

type triggerID = uint64

// msgH is the block height at which a message was present / event has happened
type msgH = abi.ChainEpoch

// triggerH is the block height at which the listener will be notified about the
//  message (msgH+confidence)
type triggerH = abi.ChainEpoch

type eventData interface{}

// EventHandler arguments:
// `prevTs` is the previous tipset, eg the "from" tipset for a state change.
// `ts` is the event tipset, eg the tipset in which the `msg` is included.
// `curH`-`ts.Height` = `confidence`
type EventHandler func(data eventData, prevTs, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error)

// CheckFunc is used for atomicity guarantees. If the condition the callbacks
// wait for has already happened in tipset `ts`
//
// If `done` is true, timeout won't be triggered
// If `more` is false, no messages will be sent to EventHandler (RevertHandler
//  may still be called)
type CheckFunc func(ts *types.TipSet) (done bool, more bool, err error)

// Keep track of information for an event handler
type handlerInfo struct {
	confidence int
	timeout    abi.ChainEpoch

	disabled bool // TODO: GC after gcConfidence reached

	handle EventHandler
	revert RevertHandler
}

// When a change occurs, a queuedEvent is created and put into a queue
// until the required confidence is reached
type queuedEvent struct {
	trigger triggerID

	prevH abi.ChainEpoch
	h     abi.ChainEpoch
	data  eventData

	called bool
}

// Manages chain head change events, which may be forward (new tipset added to
// chain) or backward (chain branch discarded in favour of heavier branch)
type hcEvents struct {
	cs           eventAPI
	tsc          *tipSetCache
	ctx          context.Context
	gcConfidence uint64

	lastTs *types.TipSet

	lk sync.Mutex

	ctr triggerID

	triggers map[triggerID]*handlerInfo

	// maps block heights to events
	// [triggerH][msgH][event]
	confQueue map[triggerH]map[msgH][]*queuedEvent

	// [msgH][triggerH]
	revertQueue map[msgH][]triggerH

	// [timeoutH+confidence][triggerID]{calls}
	timeouts map[abi.ChainEpoch]map[triggerID]int

	messageEvents
	watcherEvents
}

func newHCEvents(ctx context.Context, cs eventAPI, tsc *tipSetCache, gcConfidence uint64) *hcEvents {
	e := hcEvents{
		ctx:          ctx,
		cs:           cs,
		tsc:          tsc,
		gcConfidence: gcConfidence,

		confQueue:   map[triggerH]map[msgH][]*queuedEvent{},
		revertQueue: map[msgH][]triggerH{},
		triggers:    map[triggerID]*handlerInfo{},
		timeouts:    map[abi.ChainEpoch]map[triggerID]int{},
	}

	e.messageEvents = newMessageEvents(ctx, &e, cs)
	e.watcherEvents = newWatcherEvents(ctx, &e, cs)

	return &e
}

// Called when there is a change to the head with tipsets to be
// reverted / applied
func (e *hcEvents) processHeadChangeEvent(rev, app []*types.TipSet) error {
	e.lk.Lock()
	defer e.lk.Unlock()

	for _, ts := range rev {
		e.handleReverts(ts)
		e.lastTs = ts
	}

	for _, ts := range app {
		// Check if the head change caused any state changes that we were
		// waiting for
		stateChanges := e.watcherEvents.checkStateChanges(e.lastTs, ts)

		// Queue up calls until there have been enough blocks to reach
		// confidence on the state changes
		for tid, data := range stateChanges {
			e.queueForConfidence(tid, data, e.lastTs, ts)
		}

		// Check if the head change included any new message calls
		newCalls, err := e.messageEvents.checkNewCalls(ts)
		if err != nil {
			return err
		}

		// Queue up calls until there have been enough blocks to reach
		// confidence on the message calls
		for tid, data := range newCalls {
			e.queueForConfidence(tid, data, nil, ts)
		}

		for at := e.lastTs.Height(); at <= ts.Height(); at++ {
			// Apply any queued events and timeouts that were targeted at the
			// current chain height
			e.applyWithConfidence(ts, at)
			e.applyTimeouts(ts)
		}

		// Update the latest known tipset
		e.lastTs = ts
	}

	return nil
}

func (e *hcEvents) handleReverts(ts *types.TipSet) {
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
				log.Errorf("reverting chain trigger (@H %d, triggered @ %d) failed: %s", ts.Height(), triggerH, err)
			}
		}
		delete(e.confQueue[triggerH], ts.Height())
	}
	delete(e.revertQueue, ts.Height())
}

// Queue up events until the chain has reached a height that reflects the
// desired confidence
func (e *hcEvents) queueForConfidence(trigID uint64, data eventData, prevTs, ts *types.TipSet) {
	trigger := e.triggers[trigID]

	prevH := NoHeight
	if prevTs != nil {
		prevH = prevTs.Height()
	}
	appliedH := ts.Height()

	triggerH := appliedH + abi.ChainEpoch(trigger.confidence)

	byOrigH, ok := e.confQueue[triggerH]
	if !ok {
		byOrigH = map[abi.ChainEpoch][]*queuedEvent{}
		e.confQueue[triggerH] = byOrigH
	}

	byOrigH[appliedH] = append(byOrigH[appliedH], &queuedEvent{
		trigger: trigID,
		prevH:   prevH,
		h:       appliedH,
		data:    data,
	})

	e.revertQueue[appliedH] = append(e.revertQueue[appliedH], triggerH)
}

// Apply any events that were waiting for this chain height for confidence
func (e *hcEvents) applyWithConfidence(ts *types.TipSet, height abi.ChainEpoch) {
	byOrigH, ok := e.confQueue[height]
	if !ok {
		return // no triggers at this height
	}

	for origH, events := range byOrigH {
		triggerTs, err := e.tsc.get(origH)
		if err != nil {
			log.Errorf("events: applyWithConfidence didn't find tipset for event; wanted %d; current %d", origH, height)
		}

		for _, event := range events {
			if event.called {
				continue
			}

			trigger := e.triggers[event.trigger]
			if trigger.disabled {
				continue
			}

			// Previous tipset - this is relevant for example in a state change
			// from one tipset to another
			var prevTs *types.TipSet
			if event.prevH != NoHeight {
				prevTs, err = e.tsc.get(event.prevH)
				if err != nil {
					log.Errorf("events: applyWithConfidence didn't find tipset for previous event; wanted %d; current %d", event.prevH, height)
					continue
				}
			}

			more, err := trigger.handle(event.data, prevTs, triggerTs, height)
			if err != nil {
				log.Errorf("chain trigger (@H %d, triggered @ %d) failed: %s", origH, height, err)
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

// Apply any timeouts that expire at this height
func (e *hcEvents) applyTimeouts(ts *types.TipSet) {
	triggers, ok := e.timeouts[ts.Height()]
	if !ok {
		return // nothing to do
	}

	for triggerID, calls := range triggers {
		if calls > 0 {
			continue // don't timeout if the method was called
		}
		trigger := e.triggers[triggerID]
		if trigger.disabled {
			continue
		}

		timeoutTs, err := e.tsc.get(ts.Height() - abi.ChainEpoch(trigger.confidence))
		if err != nil {
			log.Errorf("events: applyTimeouts didn't find tipset for event; wanted %d; current %d", ts.Height()-abi.ChainEpoch(trigger.confidence), ts.Height())
		}

		more, err := trigger.handle(nil, nil, timeoutTs, ts.Height())
		if err != nil {
			log.Errorf("chain trigger (call @H %d, called @ %d) failed: %s", timeoutTs.Height(), ts.Height(), err)
			continue // don't revert failed calls
		}

		trigger.disabled = !more // allows messages after timeout
	}
}

// Listen for an event
// - CheckFunc: immediately checks if the event already occurred
// - EventHandler: called when the event has occurred, after confidence tipsets
// - RevertHandler: called if the chain head changes causing the event to revert
// - confidence: wait this many tipsets before calling EventHandler
// - timeout: at this chain height, timeout on waiting for this event
func (e *hcEvents) onHeadChanged(check CheckFunc, hnd EventHandler, rev RevertHandler, confidence int, timeout abi.ChainEpoch) (triggerID, error) {
	e.lk.Lock()
	defer e.lk.Unlock()

	// Check if the event has already occurred
	ts, err := e.tsc.best()
	if err != nil {
		return 0, xerrors.Errorf("error getting best tipset: %w", err)
	}
	done, more, err := check(ts)
	if err != nil {
		return 0, xerrors.Errorf("called check error (h: %d): %w", ts.Height(), err)
	}
	if done {
		timeout = NoTimeout
	}

	// Create a trigger for the event
	id := e.ctr
	e.ctr++

	e.triggers[id] = &handlerInfo{
		confidence: confidence,
		timeout:    timeout + abi.ChainEpoch(confidence),

		disabled: !more,

		handle: hnd,
		revert: rev,
	}

	// If there's a timeout, set up a timeout check at that height
	if timeout != NoTimeout {
		if e.timeouts[timeout+abi.ChainEpoch(confidence)] == nil {
			e.timeouts[timeout+abi.ChainEpoch(confidence)] = map[uint64]int{}
		}
		e.timeouts[timeout+abi.ChainEpoch(confidence)][id] = 0
	}

	return id, nil
}

// headChangeAPI is used to allow the composed event APIs to call back to hcEvents
// to listen for changes
type headChangeAPI interface {
	onHeadChanged(check CheckFunc, hnd EventHandler, rev RevertHandler, confidence int, timeout abi.ChainEpoch) (triggerID, error)
}

// watcherEvents watches for a state change
type watcherEvents struct {
	ctx   context.Context
	cs    eventAPI
	hcAPI headChangeAPI

	lk       sync.RWMutex
	matchers map[triggerID]StateMatchFunc
}

func newWatcherEvents(ctx context.Context, hcAPI headChangeAPI, cs eventAPI) watcherEvents {
	return watcherEvents{
		ctx:      ctx,
		cs:       cs,
		hcAPI:    hcAPI,
		matchers: make(map[triggerID]StateMatchFunc),
	}
}

// Run each of the matchers against the previous and current state to see if
// there's a change
func (we *watcherEvents) checkStateChanges(oldState, newState *types.TipSet) map[triggerID]eventData {
	we.lk.RLock()
	defer we.lk.RUnlock()

	res := make(map[triggerID]eventData)
	for tid, matchFn := range we.matchers {
		ok, data, err := matchFn(oldState, newState)
		if err != nil {
			log.Errorf("event diff fn failed: %s", err)
			continue
		}

		if ok {
			res[tid] = data
		}
	}
	return res
}

// StateChange represents a change in state
type StateChange interface{}

// StateChangeHandler arguments:
// `oldTs` is the state "from" tipset
// `newTs` is the state "to" tipset
// `states` is the change in state
// `curH`-`ts.Height` = `confidence`
type StateChangeHandler func(oldTs, newTs *types.TipSet, states StateChange, curH abi.ChainEpoch) (more bool, err error)

type StateMatchFunc func(oldTs, newTs *types.TipSet) (bool, StateChange, error)

// StateChanged registers a callback which is triggered when a specified state
// change occurs or a timeout is reached.
//
// * `CheckFunc` callback is invoked immediately with a recent tipset, it
//    returns two booleans - `done`, and `more`.
//
//   * `done` should be true when some on-chain state change we are waiting
//     for has happened. When `done` is set to true, timeout trigger is disabled.
//
//   * `more` should be false when we don't want to receive new notifications
//     through StateChangeHandler. Note that notifications may still be delivered to
//     RevertHandler
//
// * `StateChangeHandler` is called when the specified state change was observed
//    on-chain, and a confidence threshold was reached, or the specified `timeout`
//    height was reached with no state change observed. When this callback is
//    invoked on a timeout, `oldState` and `newState` are set to nil.
//    This callback returns a boolean specifying whether further notifications
//    should be sent, like `more` return param from `CheckFunc` above.
//
// * `RevertHandler` is called after apply handler, when we drop the tipset
//    containing the message. The tipset passed as the argument is the tipset
//    that is being dropped. Note that the event dropped may be re-applied
//    in a different tipset in small amount of time.
//
// * `StateMatchFunc` is called against each tipset state. If there is a match,
//   the state change is queued up until the confidence interval has elapsed (and
//   `StateChangeHandler` is called)
func (we *watcherEvents) StateChanged(check CheckFunc, scHnd StateChangeHandler, rev RevertHandler, confidence int, timeout abi.ChainEpoch, mf StateMatchFunc) error {
	hnd := func(data eventData, prevTs, ts *types.TipSet, height abi.ChainEpoch) (bool, error) {
		states, ok := data.(StateChange)
		if data != nil && !ok {
			panic("expected StateChange")
		}

		return scHnd(prevTs, ts, states, height)
	}

	id, err := we.hcAPI.onHeadChanged(check, hnd, rev, confidence, timeout)
	if err != nil {
		return err
	}

	we.lk.Lock()
	defer we.lk.Unlock()
	we.matchers[id] = mf

	return nil
}

// messageEvents watches for message calls to actors
type messageEvents struct {
	ctx   context.Context
	cs    eventAPI
	hcAPI headChangeAPI

	lk       sync.RWMutex
	matchers map[triggerID][]MsgMatchFunc
}

func newMessageEvents(ctx context.Context, hcAPI headChangeAPI, cs eventAPI) messageEvents {
	return messageEvents{
		ctx:      ctx,
		cs:       cs,
		hcAPI:    hcAPI,
		matchers: map[triggerID][]MsgMatchFunc{},
	}
}

// Check if there are any new actor calls
func (me *messageEvents) checkNewCalls(ts *types.TipSet) (map[triggerID]eventData, error) {
	pts, err := me.cs.ChainGetTipSet(me.ctx, ts.Parents()) // we actually care about messages in the parent tipset here
	if err != nil {
		log.Errorf("getting parent tipset in checkNewCalls: %s", err)
		return nil, err
	}

	me.lk.RLock()
	defer me.lk.RUnlock()

	res := make(map[triggerID]eventData)
	me.messagesForTs(pts, func(msg *types.Message) {
		// TODO: provide receipts

		for tid, matchFns := range me.matchers {
			var matched bool
			var once bool
			for _, matchFn := range matchFns {
				matchOne, ok, err := matchFn(msg)
				if err != nil {
					log.Errorf("event matcher failed: %s", err)
					continue
				}
				matched = ok
				once = matchOne

				if matched {
					break
				}
			}

			if matched {
				res[tid] = msg
				if once {
					break
				}
			}
		}
	})

	return res, nil
}

// Get the messages in a tipset
func (me *messageEvents) messagesForTs(ts *types.TipSet, consume func(*types.Message)) {
	seen := map[cid.Cid]struct{}{}

	for _, tsb := range ts.Blocks() {

		msgs, err := me.cs.ChainGetBlockMessages(context.TODO(), tsb.Cid())
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

// MsgHandler arguments:
// `ts` is the tipset, in which the `msg` is included.
// `curH`-`ts.Height` = `confidence`
type MsgHandler func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error)

type MsgMatchFunc func(msg *types.Message) (matchOnce bool, matched bool, err error)

// Called registers a callback which is triggered when a specified method is
//  called on an actor, or a timeout is reached.
//
// * `CheckFunc` callback is invoked immediately with a recent tipset, it
//    returns two booleans - `done`, and `more`.
//
//   * `done` should be true when some on-chain action we are waiting for has
//     happened. When `done` is set to true, timeout trigger is disabled.
//
//   * `more` should be false when we don't want to receive new notifications
//     through MsgHandler. Note that notifications may still be delivered to
//     RevertHandler
//
// * `MsgHandler` is called when the specified event was observed on-chain,
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
//
// * `MsgMatchFunc` is called against each message. If there is a match, the
//   message is queued up until the confidence interval has elapsed (and
//   `MsgHandler` is called)
func (me *messageEvents) Called(check CheckFunc, msgHnd MsgHandler, rev RevertHandler, confidence int, timeout abi.ChainEpoch, mf MsgMatchFunc) error {
	hnd := func(data eventData, prevTs, ts *types.TipSet, height abi.ChainEpoch) (bool, error) {
		msg, ok := data.(*types.Message)
		if data != nil && !ok {
			panic("expected msg")
		}

		rec, err := me.cs.StateGetReceipt(me.ctx, msg.Cid(), ts.Key())
		if err != nil {
			return false, err
		}

		return msgHnd(msg, rec, ts, height)
	}

	id, err := me.hcAPI.onHeadChanged(check, hnd, rev, confidence, timeout)
	if err != nil {
		return err
	}

	me.lk.Lock()
	defer me.lk.Unlock()
	me.matchers[id] = append(me.matchers[id], mf)

	return nil
}

// Convenience function for checking and matching messages
func (me *messageEvents) CalledMsg(ctx context.Context, hnd MsgHandler, rev RevertHandler, confidence int, timeout abi.ChainEpoch, msg types.ChainMsg) error {
	return me.Called(me.CheckMsg(ctx, msg, hnd), hnd, rev, confidence, timeout, me.MatchMsg(msg.VMMessage()))
}
