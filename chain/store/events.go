package store

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/types"
)

// `curH`-`ts.Height` = `confidence`
type HeightHandler func(ts *types.TipSet, curH uint64) error
type RevertHandler func(ts *types.TipSet) error

type heightHandler struct {
	confidence int

	handle HeightHandler
	revert RevertHandler
}

type eventChainStore interface {
	SubscribeHeadChanges(f func(rev, app []*types.TipSet) error)

	GetHeaviestTipSet() *types.TipSet
	MessagesForBlock(b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error)
}

type Events struct {
	cs eventChainStore

	tsc *tipSetCache
	lk  sync.Mutex

	heightEvents
	calledEvents
}

func NewEvents(cs eventChainStore) *Events {
	gcConfidence := 2 * build.ForkLengthThreshold

	tsc := newTSCache(gcConfidence)

	e := &Events{
		cs: cs,

		tsc: tsc,

		heightEvents: heightEvents{
			tsc:          tsc,
			gcConfidence: uint64(gcConfidence),

			heightTriggers:   map[uint64]*heightHandler{},
			htTriggerHeights: map[uint64][]uint64{},
			htHeights:        map[uint64][]uint64{},
		},

		calledEvents: calledEvents{
			cs:           cs,
			tsc:          tsc,
			gcConfidence: uint64(gcConfidence),

			confQueue:   map[triggerH]map[msgH][]*queuedEvent{},
			revertQueue: map[msgH][]triggerH{},
			triggers:    map[triggerId]*callHandler{},
			callTuples:  map[callTuple][]triggerId{},
			timeouts:    map[uint64]map[triggerId]int{},
		},
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
