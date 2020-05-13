package events

import (
	"context"
	"sync"
	"time"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("events")

// `curH`-`ts.Height` = `confidence`
type HeightHandler func(ctx context.Context, ts *types.TipSet, curH abi.ChainEpoch) error
type RevertHandler func(ctx context.Context, ts *types.TipSet) error

type heightHandler struct {
	confidence int
	called     bool

	handle HeightHandler
	revert RevertHandler
}

type eventApi interface {
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	StateGetReceipt(context.Context, cid.Cid, types.TipSetKey) (*types.MessageReceipt, error)

	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) // optional / for CalledMsg
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

func NewEvents(ctx context.Context, api eventApi) *Events {
	gcConfidence := 2 * build.ForkLengthThreshold

	tsc := newTSCache(gcConfidence, api.ChainGetTipSetByHeight)

	e := &Events{
		api: api,

		tsc: tsc,

		heightEvents: heightEvents{
			tsc:          tsc,
			ctx:          ctx,
			gcConfidence: abi.ChainEpoch(gcConfidence),

			heightTriggers:   map[uint64]*heightHandler{},
			htTriggerHeights: map[abi.ChainEpoch][]uint64{},
			htHeights:        map[abi.ChainEpoch][]uint64{},
		},

		calledEvents: calledEvents{
			cs:           api,
			tsc:          tsc,
			ctx:          ctx,
			gcConfidence: uint64(gcConfidence),

			confQueue:   map[triggerH]map[msgH][]*queuedEvent{},
			revertQueue: map[msgH][]triggerH{},
			triggers:    map[triggerId]*callHandler{},
			matchers:    map[triggerId][]MatchFunc{},
			timeouts:    map[abi.ChainEpoch]map[triggerId]int{},
		},
	}

	e.ready.Add(1)

	go e.listenHeadChanges(ctx)

	e.ready.Wait()

	// TODO: cleanup/gc goroutine

	return e
}

func (e *Events) listenHeadChanges(ctx context.Context) {
	for {
		if err := e.listenHeadChangesOnce(ctx); err != nil {
			log.Errorf("listen head changes errored: %s", err)
		} else {
			log.Warn("listenHeadChanges quit")
		}
		if ctx.Err() != nil {
			log.Warnf("not restarting listenHeadChanges: context error: %s", ctx.Err())
			return
		}
		time.Sleep(time.Second)
		log.Info("restarting listenHeadChanges")
	}
}

func (e *Events) listenHeadChangesOnce(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	notifs, err := e.api.ChainNotify(ctx)
	if err != nil {
		// TODO: retry
		return xerrors.Errorf("listenHeadChanges ChainNotify call failed: %w", err)
	}

	cur, ok := <-notifs // TODO: timeout?
	if !ok {
		return xerrors.Errorf("notification channel closed")
	}

	if len(cur) != 1 {
		return xerrors.Errorf("unexpected initial head notification length: %d", len(cur))
	}

	if cur[0].Type != store.HCCurrent {
		return xerrors.Errorf("expected first head notification type to be 'current', was '%s'", cur[0].Type)
	}

	if err := e.tsc.add(cur[0].Val); err != nil {
		log.Warn("tsc.add: adding current tipset failed: %w", err)
	}

	e.readyOnce.Do(func() {
		e.at = cur[0].Val.Height()

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

	return nil
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
