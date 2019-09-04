package store

import (
	"github.com/ipfs/go-cid"
	"math"
	"sync"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
)

// continous
// with reverts
// no timout

// `ts` is the tipset, in which the `msg` is included.
// `curH`-`ts.Height` = `confidence`
type CalledHandler func(msg *types.Message, ts *types.TipSet, curH uint64) error

// CheckFunc is used before one-shoot callbacks for atomicity
// guarantees. If the condition the callbacks wait for has already happened in
// tipset `ts`, this function MUST return true
type CheckFunc func(ts *types.TipSet) (bool, error)

type callHandler struct {
	confidence int
	timeout    uint64

	handle CalledHandler
	revert RevertHandler
}

type queuedEvent struct {
	trigger uint64

	h   uint64
	msg *types.Message
}

type calledEvents struct {
	cs  eventChainStore
	tsc *tipSetCache

	lk sync.Mutex

	ctr uint64

	// maps block heights to events
	// [triggerH][msgH][event]
	confQueue map[uint64]map[uint64][]queuedEvent

	// [msgH][triggerH]
	revertQueue map[uint64][]uint64

	triggers   map[uint64]callHandler
	callTuples map[callTuple][]uint64
}

type callTuple struct {
	actor  address.Address
	method uint64
}

func (e *calledEvents) headChangeCalled(rev, app []*types.TipSet) error {
	for _, ts := range rev {
		e.handleReverts(ts)
	}

	for _, ts := range app {
		// called triggers

		err := e.checkNewCalls(ts)
		if err != nil {
			return err
		}

		e.applyWithConfidence(ts)
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
			trigger := e.triggers[event.trigger]
			if err := trigger.revert(ts); err != nil {
				log.Errorf("reverting chain trigger (call %s.%d() @H %d, called @ %d) failed: %s", event.msg.To, event.msg.Method, ts.Height(), triggerH, err)
			}
		}
	}
}

func (e *calledEvents) checkNewCalls(ts *types.TipSet) error {
	return e.messagesForTs(ts, func(msg *types.Message) {
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
		byOrigH = map[uint64][]queuedEvent{}
		e.confQueue[triggerH] = byOrigH
	}

	byOrigH[ts.Height()] = append(byOrigH[ts.Height()], queuedEvent{
		trigger: triggerId,
		h:       ts.Height(),
		msg:     msg,
	})
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
			if err := trigger.handle(event.msg, triggerTs, ts.Height()); err != nil {
				log.Errorf("chain trigger (call %s.%d() @H %d, called @ %d) failed: %s", event.msg.To, event.msg.Method, origH, ts.Height(), err)
				continue // don't revert failed calls
			}

			e.revertQueue[origH] = append(e.revertQueue[origH], ts.Height())
		}
	}
}

func (e *calledEvents) messagesForTs(ts *types.TipSet, consume func(*types.Message)) error {
	seen := map[cid.Cid]struct{}{}

	for _, tsb := range ts.Blocks() {
		bmsgs, smsgs, err := e.cs.MessagesForBlock(tsb)
		if err != nil {
			return err
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

	return nil
}

func (e *calledEvents) Called(check CheckFunc, hnd CalledHandler, rev RevertHandler, confidence int, actor address.Address, method uint64) error {
	e.lk.Lock()
	defer e.lk.Unlock()

	// TODO: this should use older tipset, and take reverts into account
	done, err := check(e.tsc.best())
	if err != nil {
		return err
	}
	if done {
		// Already happened, don't bother registering callback
		return nil
	}

	id := e.ctr
	e.ctr++

	e.triggers[id] = callHandler{
		confidence: confidence,
		timeout:    math.MaxUint64, // TODO

		handle: hnd,
		revert: rev,
	}

	ct := callTuple{
		actor:  actor,
		method: method,
	}

	e.callTuples[ct] = append(e.callTuples[ct], id)
	return nil
}
