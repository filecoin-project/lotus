package events

import (
	"context"
	"math"
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
)

const NoTimeout = math.MaxInt64
const NoHeight = abi.ChainEpoch(-1)

type triggerID = uint64

// msgH is the block height at which a message was present / event has happened
type msgH = abi.ChainEpoch

// triggerH is the block height at which the listener will be notified about the
//
//	message (msgH+confidence)
type triggerH = abi.ChainEpoch

type eventData interface{}

// EventHandler arguments:
// `prevTs` is the previous tipset, eg the "from" tipset for a state change.
// `ts` is the event tipset, eg the tipset in which the `msg` is included.
// `curH`-`ts.Height` = `confidence`
type EventHandler func(ctx context.Context, data eventData, prevTs, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error)

// CheckFunc is used for atomicity guarantees. If the condition the callbacks
// wait for has already happened in tipset `ts`
//
// If `done` is true, timeout won't be triggered
// If `more` is false, no messages will be sent to EventHandler (RevertHandler
//
//	may still be called)
type CheckFunc func(ctx context.Context, ts *types.TipSet) (done bool, more bool, err error)

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
	data    eventData

	prevTipset, tipset *types.TipSet

	called bool
}

// Manages chain head change events, which may be forward (new tipset added to
// chain) or backward (chain branch discarded in favour of heavier branch)
type hcEvents struct {
	cs EventHelperAPI

	lk     sync.Mutex
	lastTs *types.TipSet

	ctr triggerID

	// TODO: get rid of trigger IDs and just use pointers as keys.
	triggers map[triggerID]*handlerInfo

	// TODO: instead of scheduling events in the future, look at the chain in the past. We can sip the "confidence" queue entirely.
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

func newHCEvents(api EventHelperAPI, obs *observer) *hcEvents {
	e := &hcEvents{
		cs:          api,
		confQueue:   map[triggerH]map[msgH][]*queuedEvent{},
		revertQueue: map[msgH][]triggerH{},
		triggers:    map[triggerID]*handlerInfo{},
		timeouts:    map[abi.ChainEpoch]map[triggerID]int{},
	}

	e.messageEvents = newMessageEvents(e, api)
	e.watcherEvents = newWatcherEvents(e, api)

	// We need to take the lock as the observer could immediately try calling us.
	e.lk.Lock()
	e.lastTs = obs.Observe((*hcEventsObserver)(e))
	e.lk.Unlock()

	return e
}

type hcEventsObserver hcEvents

func (e *hcEventsObserver) Apply(ctx context.Context, from, to *types.TipSet) error {
	e.lk.Lock()
	defer e.lk.Unlock()

	defer func() { e.lastTs = to }()

	// Check if the head change caused any state changes that we were
	// waiting for
	stateChanges := e.checkStateChanges(from, to)

	// Queue up calls until there have been enough blocks to reach
	// confidence on the state changes
	for tid, data := range stateChanges {
		e.queueForConfidence(tid, data, from, to)
	}

	// Check if the head change included any new message calls
	newCalls := e.checkNewCalls(ctx, from, to)

	// Queue up calls until there have been enough blocks to reach
	// confidence on the message calls
	for tid, calls := range newCalls {
		for _, data := range calls {
			e.queueForConfidence(tid, data, nil, to)
		}
	}

	for at := from.Height() + 1; at <= to.Height(); at++ {
		// Apply any queued events and timeouts that were targeted at the
		// current chain height
		e.applyWithConfidence(ctx, at)
		e.applyTimeouts(ctx, at, to)
	}
	return nil
}

func (e *hcEventsObserver) Revert(ctx context.Context, from, to *types.TipSet) error {
	e.lk.Lock()
	defer e.lk.Unlock()

	defer func() { e.lastTs = to }()

	reverts, ok := e.revertQueue[from.Height()]
	if !ok {
		return nil // nothing to do
	}

	for _, triggerH := range reverts {
		toRevert := e.confQueue[triggerH][from.Height()]
		for _, event := range toRevert {
			if !event.called {
				continue // event wasn't apply()-ied yet
			}

			trigger := e.triggers[event.trigger]

			if err := trigger.revert(ctx, from); err != nil {
				log.Errorf("reverting chain trigger (@H %d, triggered @ %d) failed: %s", from.Height(), triggerH, err)
			}
		}
		delete(e.confQueue[triggerH], from.Height())
	}
	delete(e.revertQueue, from.Height())
	return nil
}

// Queue up events until the chain has reached a height that reflects the
// desired confidence
func (e *hcEventsObserver) queueForConfidence(trigID uint64, data eventData, prevTs, ts *types.TipSet) {
	trigger := e.triggers[trigID]

	appliedH := ts.Height()

	triggerH := appliedH + abi.ChainEpoch(trigger.confidence)

	byOrigH, ok := e.confQueue[triggerH]
	if !ok {
		byOrigH = map[abi.ChainEpoch][]*queuedEvent{}
		e.confQueue[triggerH] = byOrigH
	}

	byOrigH[appliedH] = append(byOrigH[appliedH], &queuedEvent{
		trigger:    trigID,
		data:       data,
		tipset:     ts,
		prevTipset: prevTs,
	})

	e.revertQueue[appliedH] = append(e.revertQueue[appliedH], triggerH)
}

// Apply any events that were waiting for this chain height for confidence
func (e *hcEventsObserver) applyWithConfidence(ctx context.Context, height abi.ChainEpoch) {
	byOrigH, ok := e.confQueue[height]
	if !ok {
		return // no triggers at this height
	}

	for origH, events := range byOrigH {
		for _, event := range events {
			if event.called {
				continue
			}

			trigger := e.triggers[event.trigger]
			if trigger.disabled {
				continue
			}

			more, err := trigger.handle(ctx, event.data, event.prevTipset, event.tipset, height)
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
func (e *hcEventsObserver) applyTimeouts(ctx context.Context, at abi.ChainEpoch, ts *types.TipSet) {
	triggers, ok := e.timeouts[at]
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

		// This should be cached.
		timeoutTs, err := e.cs.ChainGetTipSetAfterHeight(ctx, at-abi.ChainEpoch(trigger.confidence), ts.Key())
		if err != nil {
			log.Errorf("events: applyTimeouts didn't find tipset for event; wanted %d; current %d", at-abi.ChainEpoch(trigger.confidence), at)
		}

		more, err := trigger.handle(ctx, nil, nil, timeoutTs, at)
		if err != nil {
			log.Errorf("chain trigger (call @H %d, called @ %d) failed: %s", timeoutTs.Height(), at, err)
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
func (e *hcEvents) onHeadChanged(ctx context.Context, check CheckFunc, hnd EventHandler, rev RevertHandler, confidence int, timeout abi.ChainEpoch) (triggerID, error) {
	e.lk.Lock()
	defer e.lk.Unlock()

	// Check if the event has already occurred
	done, more, err := check(ctx, e.lastTs)
	if err != nil {
		return 0, xerrors.Errorf("called check error (h: %d): %w", e.lastTs.Height(), err)
	}
	if done {
		timeout = NoTimeout
	}

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
	onHeadChanged(ctx context.Context, check CheckFunc, hnd EventHandler, rev RevertHandler, confidence int, timeout abi.ChainEpoch) (triggerID, error)
}

// watcherEvents watches for a state change
type watcherEvents struct {
	cs    EventHelperAPI
	hcAPI headChangeAPI

	lk       sync.RWMutex
	matchers map[triggerID]StateMatchFunc
}

func newWatcherEvents(hcAPI headChangeAPI, cs EventHelperAPI) watcherEvents {
	return watcherEvents{
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
//   - `CheckFunc` callback is invoked immediately with a recent tipset, it
//     returns two booleans - `done`, and `more`.
//
//   - `done` should be true when some on-chain state change we are waiting
//     for has happened. When `done` is set to true, timeout trigger is disabled.
//
//   - `more` should be false when we don't want to receive new notifications
//     through StateChangeHandler. Note that notifications may still be delivered to
//     RevertHandler
//
//   - `StateChangeHandler` is called when the specified state change was observed
//     on-chain, and a confidence threshold was reached, or the specified `timeout`
//     height was reached with no state change observed. When this callback is
//     invoked on a timeout, `oldTs` and `states` are set to nil.
//     This callback returns a boolean specifying whether further notifications
//     should be sent, like `more` return param from `CheckFunc` above.
//
//   - `RevertHandler` is called after apply handler, when we drop the tipset
//     containing the message. The tipset passed as the argument is the tipset
//     that is being dropped. Note that the event dropped may be re-applied
//     in a different tipset in small amount of time.
//
//   - `StateMatchFunc` is called against each tipset state. If there is a match,
//     the state change is queued up until the confidence interval has elapsed (and
//     `StateChangeHandler` is called)
func (we *watcherEvents) StateChanged(check CheckFunc, scHnd StateChangeHandler, rev RevertHandler, confidence int, timeout abi.ChainEpoch, mf StateMatchFunc) error {
	hnd := func(ctx context.Context, data eventData, prevTs, ts *types.TipSet, height abi.ChainEpoch) (bool, error) {
		states, ok := data.(StateChange)
		if data != nil && !ok {
			panic("expected StateChange")
		}

		return scHnd(prevTs, ts, states, height)
	}

	id, err := we.hcAPI.onHeadChanged(context.TODO(), check, hnd, rev, confidence, timeout)
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
	cs    EventHelperAPI
	hcAPI headChangeAPI

	lk       sync.RWMutex
	matchers map[triggerID]MsgMatchFunc
}

func newMessageEvents(hcAPI headChangeAPI, cs EventHelperAPI) messageEvents {
	return messageEvents{
		cs:       cs,
		hcAPI:    hcAPI,
		matchers: make(map[triggerID]MsgMatchFunc),
	}
}

// Check if there are any new actor calls
func (me *messageEvents) checkNewCalls(ctx context.Context, from, to *types.TipSet) map[triggerID][]eventData {
	me.lk.RLock()
	defer me.lk.RUnlock()

	// For each message in the tipset
	res := make(map[triggerID][]eventData)
	me.messagesForTs(from, func(msg *types.Message) {
		// TODO: provide receipts

		// Run each trigger's matcher against the message
		for tid, matchFn := range me.matchers {
			matched, err := matchFn(msg)
			if err != nil {
				log.Errorf("event matcher failed: %s", err)
				continue
			}

			// If there was a match, include the message in the results for the
			// trigger
			if matched {
				res[tid] = append(res[tid], msg)
			}
		}
	})

	return res
}

// Get the messages in a tipset
func (me *messageEvents) messagesForTs(ts *types.TipSet, consume func(*types.Message)) {
	seen := map[cid.Cid]struct{}{}

	for i, tsb := range ts.Cids() {
		msgs, err := me.cs.ChainGetBlockMessages(context.TODO(), tsb)
		if err != nil {
			log.Errorf("messagesForTs MessagesForBlock failed (ts.H=%d, Bcid:%s, B.Mcid:%s): %s",
				ts.Height(), tsb, ts.Blocks()[i].Messages, err)
			continue
		}
		for i, c := range msgs.Cids {
			// We iterate over the CIDs to avoid having to recompute them.
			_, ok := seen[c]
			if ok {
				continue
			}
			seen[c] = struct{}{}
			if i < len(msgs.BlsMessages) {
				consume(msgs.BlsMessages[i])
			} else {
				consume(&msgs.SecpkMessages[i-len(msgs.BlsMessages)].Message)
			}
		}
	}
}

// MsgHandler arguments:
// `ts` is the tipset, in which the `msg` is included.
// `curH`-`ts.Height` = `confidence`
type MsgHandler func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error)

type MsgMatchFunc func(msg *types.Message) (matched bool, err error)

// Called registers a callback which is triggered when a specified method is
//
//		called on an actor, or a timeout is reached.
//
//	  - `CheckFunc` callback is invoked immediately with a recent tipset, it
//	    returns two booleans - `done`, and `more`.
//
//	  - `done` should be true when some on-chain action we are waiting for has
//	    happened. When `done` is set to true, timeout trigger is disabled.
//
//	  - `more` should be false when we don't want to receive new notifications
//	    through MsgHandler. Note that notifications may still be delivered to
//	    RevertHandler
//
//	  - `MsgHandler` is called when the specified event was observed on-chain,
//	    and a confidence threshold was reached, or the specified `timeout` height
//	    was reached with no events observed. When this callback is invoked on a
//	    timeout, `msg` is set to nil. This callback returns a boolean specifying
//	    whether further notifications should be sent, like `more` return param
//	    from `CheckFunc` above.
//
//	  - `RevertHandler` is called after apply handler, when we drop the tipset
//	    containing the message. The tipset passed as the argument is the tipset
//	    that is being dropped. Note that the message dropped may be re-applied
//	    in a different tipset in small amount of time.
//
//	  - `MsgMatchFunc` is called against each message. If there is a match, the
//	    message is queued up until the confidence interval has elapsed (and
//	    `MsgHandler` is called)
func (me *messageEvents) Called(ctx context.Context, check CheckFunc, msgHnd MsgHandler, rev RevertHandler, confidence int, timeout abi.ChainEpoch, mf MsgMatchFunc) error {
	hnd := func(ctx context.Context, data eventData, prevTs, ts *types.TipSet, height abi.ChainEpoch) (bool, error) {
		msg, ok := data.(*types.Message)
		if data != nil && !ok {
			panic("expected msg")
		}

		ml, err := me.cs.StateSearchMsg(ctx, ts.Key(), msg.Cid(), stmgr.LookbackNoLimit, true)
		if err != nil {
			return false, err
		}

		if ml == nil {
			return msgHnd(msg, nil, ts, height)
		}

		return msgHnd(msg, &ml.Receipt, ts, height)
	}

	id, err := me.hcAPI.onHeadChanged(ctx, check, hnd, rev, confidence, timeout)
	if err != nil {
		return xerrors.Errorf("on head changed error: %w", err)
	}

	me.lk.Lock()
	defer me.lk.Unlock()
	me.matchers[id] = mf

	return nil
}

// Convenience function for checking and matching messages
func (me *messageEvents) CalledMsg(ctx context.Context, hnd MsgHandler, rev RevertHandler, confidence int, timeout abi.ChainEpoch, msg types.ChainMsg) error {
	return me.Called(ctx, me.CheckMsg(msg, hnd), hnd, rev, confidence, timeout, me.MatchMsg(msg.VMMessage()))
}
