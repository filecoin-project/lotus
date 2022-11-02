package modules

import (
	"context"
	"time"

	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type EventAPI struct {
	fx.In

	full.ChainAPI
	full.StateAPI
}

var _ events.EventAPI = &EventAPI{}

func EthEvent(cfg config.ActorEventConfig) func(helpers.MetricsCtx, fx.Lifecycle, *store.ChainStore, EventAPI, *messagepool.MessagePool) (*full.EthEvent, error) {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, cs *store.ChainStore, evapi EventAPI, mp *messagepool.MessagePool) (*full.EthEvent, error) {
		ee := &full.EthEvent{}

		if !cfg.EnableRealTimeFilterAPI && !cfg.EnableHistoricFilterAPI {
			// all event functionality is disabled
			return ee, nil
		}

		ee.FilterStore = filter.NewMemFilterStore(cfg.MaxFilters)

		// Start garbage collection for filters
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				go ee.GC(ctx, time.Duration(cfg.FilterTTL))
				return nil
			},
		})

		if cfg.EnableRealTimeFilterAPI {
			ee.EventFilterManager = &filter.EventFilterManager{
				ChainStore:       cs,
				MaxFilterResults: cfg.MaxFilterResults,
			}
			ee.TipSetFilterManager = &filter.TipSetFilterManager{
				MaxFilterResults: cfg.MaxFilterResults,
			}
			ee.MemPoolFilterManager = &filter.MemPoolFilterManager{
				MaxFilterResults: cfg.MaxFilterResults,
			}

			const ChainHeadConfidence = 1

			ctx := helpers.LifecycleCtx(mctx, lc)
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					ev, err := events.NewEventsWithConfidence(ctx, &evapi, ChainHeadConfidence)
					if err != nil {
						return err
					}
					// ignore returned tipsets
					_ = ev.Observe(ee.EventFilterManager)
					_ = ev.Observe(ee.TipSetFilterManager)

					ch, err := mp.Updates(ctx)
					if err != nil {
						return err
					}
					go ee.MemPoolFilterManager.WaitForMpoolUpdates(ctx, ch)

					return nil
				},
			})

		}

		if cfg.EnableHistoricFilterAPI {
			// TODO: enable indexer
		}

		return ee, nil
	}
}
