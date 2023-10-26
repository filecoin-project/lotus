package chainsched

import (
	"context"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("chainsched")

type NodeAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
}

type ProviderChainSched struct {
	api NodeAPI

	callbacks []UpdateFunc
	started   bool
}

func New(api NodeAPI) *ProviderChainSched {
	return &ProviderChainSched{
		api: api,
	}
}

type UpdateFunc func(ctx context.Context, revert, apply *types.TipSet) error

func (s *ProviderChainSched) AddHandler(ch UpdateFunc) error {
	if s.started {
		return xerrors.Errorf("cannot add handler after start")
	}

	s.callbacks = append(s.callbacks, ch)
	return nil
}

func (s *ProviderChainSched) Run(ctx context.Context) {
	s.started = true

	var (
		notifs <-chan []*api.HeadChange
		err    error
		gotCur bool
	)

	// not fine to panic after this point
	for {
		if notifs == nil {
			notifs, err = s.api.ChainNotify(ctx)
			if err != nil {
				log.Errorf("ChainNotify error: %+v", err)

				build.Clock.Sleep(10 * time.Second)
				continue
			}

			gotCur = false
			log.Info("restarting window post scheduler")
		}

		select {
		case changes, ok := <-notifs:
			if !ok {
				log.Warn("window post scheduler notifs channel closed")
				notifs = nil
				continue
			}

			if !gotCur {
				if len(changes) != 1 {
					log.Errorf("expected first notif to have len = 1")
					continue
				}
				chg := changes[0]
				if chg.Type != store.HCCurrent {
					log.Errorf("expected first notif to tell current ts")
					continue
				}

				ctx, span := trace.StartSpan(ctx, "ProviderChainSched.headChange")

				s.update(ctx, nil, chg.Val)

				span.End()
				gotCur = true
				continue
			}

			ctx, span := trace.StartSpan(ctx, "ProviderChainSched.headChange")

			var lowest, highest *types.TipSet = nil, nil

			for _, change := range changes {
				if change.Val == nil {
					log.Errorf("change.Val was nil")
				}
				switch change.Type {
				case store.HCRevert:
					lowest = change.Val
				case store.HCApply:
					highest = change.Val
				}
			}

			s.update(ctx, lowest, highest)

			span.End()
		case <-ctx.Done():
			return
		}
	}
}

func (s *ProviderChainSched) update(ctx context.Context, revert, apply *types.TipSet) {
	if apply == nil {
		log.Error("no new tipset in window post ProviderChainSched.update")
		return
	}

	for _, ch := range s.callbacks {
		if err := ch(ctx, revert, apply); err != nil {
			log.Errorf("handling head updates in provider chain sched: %+v", err)
		}
	}
}
