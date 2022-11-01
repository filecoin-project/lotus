package modules

import (
	"context"

	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type EventAPI struct {
	fx.In

	full.ChainAPI
	full.StateAPI
}

var _ events.EventAPI = &EventAPI{}

func EventFilterManager(mctx helpers.MetricsCtx, lc fx.Lifecycle, cs *store.ChainStore, evapi EventAPI) *filter.EventFilterManager {
	m := &filter.EventFilterManager{
		ChainStore: cs,
	}

	const ChainHeadConfidence = 1

	ctx := helpers.LifecycleCtx(mctx, lc)
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ev, err := events.NewEventsWithConfidence(ctx, &evapi, ChainHeadConfidence)
			if err != nil {
				return err
			}
			// ignore returned tipset
			_ = ev.Observe(m)
			return nil
		},
	})

	return m
}

func TipSetFilterManager(mctx helpers.MetricsCtx, lc fx.Lifecycle, evapi EventAPI) *filter.TipSetFilterManager {
	m := &filter.TipSetFilterManager{}

	const ChainHeadConfidence = 1

	ctx := helpers.LifecycleCtx(mctx, lc)
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ev, err := events.NewEventsWithConfidence(ctx, &evapi, ChainHeadConfidence)
			if err != nil {
				return err
			}
			// ignore returned tipset
			_ = ev.Observe(m)
			return nil
		},
	})

	return m
}

func MemPoolFilterManager(lc fx.Lifecycle, mp *messagepool.MessagePool) *filter.MemPoolFilterManager {
	m := &filter.MemPoolFilterManager{}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ch, err := mp.Updates(ctx)
			if err != nil {
				return err
			}

			go m.WaitForMpoolUpdates(ctx, ch)
			return nil
		},
	})

	return m
}

func FilterStore() filter.FilterStore {
	return filter.NewMemFilterStore()
}
