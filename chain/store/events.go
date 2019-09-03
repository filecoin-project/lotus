package store

import (
	"fmt"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
	"sync"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
)

// CheckFunc is used before one-shoot callbacks for atomicity
// guarantees. If the condition the callbacks wait for has already happened in
// tipset `ts`, this function MUST return true
type CheckFunc func(ts *types.TipSet) (bool, error)

// `ts` is the tipset, in which the `msg` is included.
// `curH`-`ts.Height` = `confidence`
type HandleFunc func(msg *types.Message, ts *types.TipSet, curH uint64) error
type RevertFunc func(ts *types.TipSet) error

type handler struct {
	confidence int

	handle HandleFunc
	revert RevertFunc

	msg     *types.Message
	disable bool
}

type callTuple struct {
	actor  address.Address
	method uint64
}

type eventChainStore interface {
	SubscribeHeadChanges(f func(rev, app []*types.TipSet) error)

	GetHeaviestTipSet() *types.TipSet
	MessagesForBlock(b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error)
}

type Events struct {
	cs           eventChainStore
	gcConfidence uint64

	tsc *tipSetCache
	lk  sync.Mutex

	ctr uint64

	// ChainAt

	heightTriggers map[uint64]*handler

	htTriggerHeights map[uint64][]uint64
	htHeights        map[uint64][]uint64

	// Called

	calledTriggers map[uint64]handler

	ctTriggers map[callTuple][]uint64
}

func NewEvents(cs eventChainStore) *Events {
	gcConfidence := 2 * build.ForkLengthThreshold

	e := &Events{
		cs:           cs,
		gcConfidence: uint64(gcConfidence),

		tsc: newTSCache(gcConfidence),

		heightTriggers:   map[uint64]*handler{},
		htTriggerHeights: map[uint64][]uint64{},
		htHeights:        map[uint64][]uint64{},

		calledTriggers: map[uint64]handler{},
		ctTriggers:     map[callTuple][]uint64{},
	}

	_ = e.tsc.add(cs.GetHeaviestTipSet())
	cs.SubscribeHeadChanges(e.headChange)

	// TODO: cleanup/gc goroutine

	return e
}

func (e *Events) headChange(rev, app []*types.TipSet) error {
	if len(app) == 0 {
		return xerrors.New("events.headChange expected at least one applied tipset")
	}

	e.lk.Lock()
	defer e.lk.Unlock()

	if err := e.headChangeAt(rev, app); err != nil {
		return err
	}

	return e.headChangeCalled(rev, app)
}

func (e *Events) headChangeAt(rev, app []*types.TipSet) error {
	// highest tipset is always the first (see cs.ReorgOps)
	newH := app[0].Height()

	for _, ts := range rev {
		// TODO: log error if h below gcconfidence
		// revert height-based triggers

		for _, tid := range e.htHeights[ts.Height()] {
			// don't revert if newH is above this ts
			if newH >= ts.Height() {
				if e.heightTriggers[tid].msg != nil {
					// TODO: optimization: don't revert if app[newH - ts.Height()] contains the msg
				} else {
					continue
				}
			}

			err := e.heightTriggers[tid].revert(ts)
			if err != nil {
				log.Errorf("reverting chain trigger (@H %d): %s", ts.Height(), err)
			}
		}

		if err := e.tsc.revert(ts); err != nil {
			return err
		}
	}

	for _, ts := range app {
		if err := e.tsc.add(ts); err != nil {
			return err
		}

		// height triggers

		for _, tid := range e.htTriggerHeights[ts.Height()] {
			hnd := e.heightTriggers[tid]
			if hnd.disable {
				continue
			}

			triggerH := ts.Height() - uint64(hnd.confidence)

			incTs, err := e.tsc.get(triggerH)
			if err != nil {
				return err
			}

			if err := hnd.handle(hnd.msg, incTs, ts.Height()); err != nil {
				msgInfo := ""
				if hnd.msg != nil {
					msgInfo = fmt.Sprintf("call %s(%d), ", hnd.msg.To, hnd.msg.Method)
				}
				log.Errorf("chain trigger (%s@H %d, called @ %d) failed: %s", msgInfo, triggerH, ts.Height(), err)
			}
			hnd.disable = hnd.msg != nil // special case for Called
		}
	}

	return nil
}

func (e *Events) headChangeCalled(rev, app []*types.TipSet) error {
	for _, ts := range rev {
		_ = ts
	}

	for _, ts := range app {
		// called triggers

		err := e.messagesForTs(ts, func(msg *types.Message) error {
			// TODO: do we have to verify the receipt, or are messages on chain
			//  guaranteed to be successful?

			ct := callTuple{
				actor:  msg.To,
				method: msg.Method,
			}

			triggers, ok := e.ctTriggers[ct]
			if !ok {
				return nil
			}

			for _, tid := range triggers {
				trigger := e.calledTriggers[tid]

				err := e.chainAt(trigger.handle, trigger.revert, msg, trigger.confidence, ts.Height())
				if err != nil {
					log.Errorf("chain trigger (call %s(%d), msg found @ %d) failed: %s", msg.To, msg.Method, ts.Height(), err)
					continue
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *Events) messagesForTs(ts *types.TipSet, consume func(*types.Message) error) error {
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

			if err := consume(m); err != nil {
				return err
			}
		}

		for _, m := range smsgs {
			_, ok := seen[m.Message.Cid()]
			if ok {
				continue
			}
			seen[m.Message.Cid()] = struct{}{}

			if err := consume(&m.Message); err != nil {
				return err
			}
		}
	}

	return nil
}

func (e *Events) CalledOnce(check CheckFunc, hnd HandleFunc, rev RevertFunc, confidence int, actor address.Address, method uint64) error {
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

	e.calledTriggers[id] = handler{
		confidence: confidence,

		handle: hnd,
		revert: rev,
	}

	ct := callTuple{
		actor:  actor,
		method: method,
	}

	e.ctTriggers[ct] = append(e.ctTriggers[ct], id)
	return nil
}

func (e *Events) NotCalledBy(check CheckFunc, hnd HandleFunc, rev RevertFunc, confidence int, actor address.Address, method uint64, h uint64) {
	panic("impl")
}

func (e *Events) ChainAt(hnd HandleFunc, rev RevertFunc, confidence int, h uint64) error {
	e.lk.Lock()
	defer e.lk.Unlock()

	return e.chainAt(hnd, rev, nil, confidence, h)
}

func (e *Events) chainAt(hnd HandleFunc, rev RevertFunc, msg *types.Message, confidence int, h uint64) error {
	bestH := e.tsc.best().Height()

	if bestH >= h+uint64(confidence) {
		ts, err := e.tsc.get(h)
		if err != nil {
			log.Warnf("events.ChainAt: calling HandleFunc with nil tipset, not found in cache: %s", err)
		}

		if err := hnd(msg, ts, bestH); err != nil {
			return err
		}
	}

	if bestH >= h+uint64(confidence)+e.gcConfidence {
		return nil
	}

	triggerAt := h + uint64(confidence)

	id := e.ctr
	e.ctr++

	e.heightTriggers[id] = &handler{
		confidence: confidence,

		handle: hnd,
		revert: rev,

		msg: msg,
	}

	e.htHeights[h] = append(e.htHeights[h], id)
	e.htTriggerHeights[triggerAt] = append(e.htTriggerHeights[triggerAt], id)

	return nil
}
