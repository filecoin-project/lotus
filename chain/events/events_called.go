package events

import (
	"math"
	"sync"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
)

type triggerId = uint64

// msgH is the block height at which a message was present / event has happened
type msgH = uint64

// triggerH is the block height at which the listener will be notified about the
//  message (msgH+confidence)
type triggerH = uint64

// `ts` is the tipset, in which the `msg` is included.
// `curH`-`ts.Height` = `confidence`
type CalledHandler func(msg *types.Message, ts *types.TipSet, curH uint64) (bool, error)

// CheckFunc is used for atomicity guarantees. If the condition the callbacks
// wait for has already happened in tipset `ts`
//
// If `done` is true, timeout won't be triggered
// If `more` is false, no messages will be sent to CalledHandler (RevertHandler
//  may still be called)
type CheckFunc func(ts *types.TipSet) (done bool, more bool, err error)

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
	cs           eventChainStore
	tsc          *tipSetCache
	gcConfidence uint64

	lk sync.Mutex

	ctr triggerId

	triggers   map[triggerId]*callHandler
	callTuples map[callTuple][]triggerId

	// maps block heights to events
	// [triggerH][msgH][event]
	confQueue map[triggerH]map[msgH][]*queuedEvent

	// [msgH][triggerH]
	revertQueue map[msgH][]triggerH

	// [timeoutH+confidence][triggerId]{calls}
	timeouts map[uint64]map[triggerId]int
}

type callTuple struct {
	actor  address.Address
	method uint64
}

func (e *calledEvents) headChangeCalled(rev, app []*types.TipSet) error {
	e.lk.Lock()
	defer e.lk.Unlock()

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

			if err := trigger.revert(ts); err != nil {
				log.Errorf("reverting chain trigger (call %s.%d() @H %d, called @ %d) failed: %s", event.msg.To, event.msg.Method, ts.Height(), triggerH, err)
			}
		}
		delete(e.confQueue[triggerH], ts.Height())
	}
	delete(e.revertQueue, ts.Height())
}

func (e *calledEvents) checkNewCalls(ts *types.TipSet) {
	e.messagesForTs(ts, func(msg *types.Message) {
		// TODO: do we have to verify the receipt, or are messages on chain
		//  guaranteed to be successful?

		ct := callTuple{
			actor:  msg.To,
			method: msg.Method,
		}

		triggers, ok := e.callTuples[ct]
		if !ok {
			return
		}

		for _, tid := range triggers {
			e.queueForConfidence(tid, msg, ts)
		}
	})
}

func (e *calledEvents) queueForConfidence(triggerId uint64, msg *types.Message, ts *types.TipSet) {
	trigger := e.triggers[triggerId]
	triggerH := ts.Height() + uint64(trigger.confidence)

	byOrigH, ok := e.confQueue[triggerH]
	if !ok {
		byOrigH = map[uint64][]*queuedEvent{}
		e.confQueue[triggerH] = byOrigH
	}

	byOrigH[ts.Height()] = append(byOrigH[ts.Height()], &queuedEvent{
		trigger: triggerId,
		h:       ts.Height(),
		msg:     msg,
	})

	e.revertQueue[ts.Height()] = append(e.revertQueue[ts.Height()], triggerH) // todo: dedupe?
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
			trigger := e.triggers[event.trigger]
			if trigger.disabled {
				continue
			}

			more, err := trigger.handle(event.msg, triggerTs, ts.Height())
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

		more, err := trigger.handle(nil, timeoutTs, ts.Height())
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
		bmsgs, smsgs, err := e.cs.MessagesForBlock(tsb)
		if err != nil {
			log.Errorf("messagesForTs MessagesForBlock failed (ts.H=%d, Bcid:%s, B.Mcid:%s): %s", ts.Height(), tsb.Cid(), tsb.Messages, err)
			// this is quite bad, but probably better than missing all the other updates
			continue
		}

		for _, m := range bmsgs {
			_, ok := seen[m.Cid()]
			if ok {
				continue
			}
			seen[m.Cid()] = struct{}{}

			consume(m)
		}

		for _, m := range smsgs {
			_, ok := seen[m.Message.Cid()]
			if ok {
				continue
			}
			seen[m.Message.Cid()] = struct{}{}

			consume(&m.Message)
		}
	}
}

func (e *calledEvents) Called(check CheckFunc, hnd CalledHandler, rev RevertHandler, confidence int, timeout uint64, actor address.Address, method uint64) error {
	e.lk.Lock()
	defer e.lk.Unlock()

	done, more, err := check(e.tsc.best())
	if err != nil {
		return err
	}
	if done {
		timeout = math.MaxUint64
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

	ct := callTuple{
		actor:  actor,
		method: method,
	}

	e.callTuples[ct] = append(e.callTuples[ct], id)
	if timeout != math.MaxUint64 {
		if e.timeouts[timeout+uint64(confidence)] == nil {
			e.timeouts[timeout+uint64(confidence)] = map[uint64]int{}
		}
		e.timeouts[timeout+uint64(confidence)][id] = 0
	}

	return nil
}
