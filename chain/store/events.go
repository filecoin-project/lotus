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
	cs           eventChainStore
	gcConfidence uint64

	tsc *tipSetCache
	lk  sync.Mutex

	ctr uint64

	// ChainAt

	heightTriggers map[uint64]*heightHandler

	htTriggerHeights map[uint64][]uint64
	htHeights        map[uint64][]uint64

	calledEvents
}

func NewEvents(cs eventChainStore) *Events {
	gcConfidence := 2 * build.ForkLengthThreshold

	tsc := newTSCache(gcConfidence)

	e := &Events{
		cs:           cs,
		gcConfidence: uint64(gcConfidence),

		tsc: tsc,

		heightTriggers:   map[uint64]*heightHandler{},
		htTriggerHeights: map[uint64][]uint64{},
		htHeights:        map[uint64][]uint64{},

		calledEvents: calledEvents{
			cs:  cs,
			tsc: tsc,

			confQueue:   map[uint64]map[uint64][]*queuedEvent{},
			revertQueue: map[uint64][]uint64{},
			triggers:    map[uint64]callHandler{},
			callTuples:  map[callTuple][]uint64{},
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
