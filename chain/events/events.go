package events

import (
	"context"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
)

var log = logging.Logger("events")

// `curH`-`ts.Height` = `confidence`
type HeightHandler func(ts *types.TipSet, curH uint64) error
type RevertHandler func(ts *types.TipSet) error

type heightHandler struct {
	confidence int

	handle HeightHandler
	revert RevertHandler
}

type eventApi interface {
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
}

type Events struct {
	api eventApi

	tsc *tipSetCache
	lk  sync.Mutex

	ready     sync.WaitGroup
	readyOnce sync.Once

	heightEvents
	calledEvents
}

func NewEvents(api eventApi) *Events {
	gcConfidence := 2 * build.ForkLengthThreshold

	tsc := newTSCache(gcConfidence)

	e := &Events{
		api: api,

		tsc: tsc,

		heightEvents: heightEvents{
			tsc:          tsc,
			gcConfidence: uint64(gcConfidence),

			heightTriggers:   map[uint64]*heightHandler{},
			htTriggerHeights: map[uint64][]uint64{},
			htHeights:        map[uint64][]uint64{},
		},

		calledEvents: calledEvents{
			cs:           api,
			tsc:          tsc,
			gcConfidence: uint64(gcConfidence),

			confQueue:   map[triggerH]map[msgH][]*queuedEvent{},
			revertQueue: map[msgH][]triggerH{},
			triggers:    map[triggerId]*callHandler{},
			callTuples:  map[callTuple][]triggerId{},
			timeouts:    map[uint64]map[triggerId]int{},
		},
	}

	e.ready.Add(1)

	go e.listenHeadChanges(context.TODO())

	e.ready.Wait()

	// TODO: cleanup/gc goroutine

	return e
}

func (e *Events) restartHeadChanges(ctx context.Context) {
	go func() {
		if ctx.Err() != nil {
			log.Warnf("not restarting listenHeadChanges: context error: %s", ctx.Err())
			return
		}
		time.Sleep(time.Second)
		log.Info("restarting listenHeadChanges")
		e.listenHeadChanges(ctx)
	}()
}

func (e *Events) listenHeadChanges(ctx context.Context) {
	notifs, err := e.api.ChainNotify(ctx)
	if err != nil {
		// TODO: retry
		log.Errorf("listenHeadChanges ChainNotify call failed: %s", err)
		e.restartHeadChanges(ctx)
		return
	}

	cur, ok := <-notifs // TODO: timeout?
	if !ok {
		log.Error("notification channel closed")
		e.restartHeadChanges(ctx)
		return
	}

	if len(cur) != 1 {
		log.Errorf("unexpected initial head notification length: %d", len(cur))
		e.restartHeadChanges(ctx)
		return
	}

	if cur[0].Type != store.HCCurrent {
		log.Errorf("expected first head notification type to be 'current', was '%s'", cur[0].Type)
		e.restartHeadChanges(ctx)
		return
	}

	if err := e.tsc.add(cur[0].Val); err != nil {
		log.Warn("tsc.add: adding current tipset failed: %s", err)
	}

	e.readyOnce.Do(func() {
		e.ready.Done()
	})

	for notif := range notifs {
		var rev, app []*types.TipSet
		for _, notif := range notif {
			switch notif.Type {
			case store.HCRevert:
				rev = append(rev, notif.Val)
			case store.HCApply:
				app = append(app, notif.Val)
			default:
				log.Warnf("unexpected head change notification type: '%s'", notif.Type)
			}
		}

		if err := e.headChange(rev, app); err != nil {
			log.Warnf("headChange failed: %s", err)
		}
	}

	log.Warn("listenHeadChanges loop quit")
	e.restartHeadChanges(ctx)
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
