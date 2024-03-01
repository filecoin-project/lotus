package modules

import (
	"context"
	"path/filepath"
	"time"

	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

type EventHelperAPI struct {
	fx.In

	full.ChainAPI
	full.StateAPI
}

var _ events.EventHelperAPI = &EventHelperAPI{}

func EthEventHandler(cfg config.FevmConfig) func(helpers.MetricsCtx, repo.LockedRepo, fx.Lifecycle, *filter.EventFilterManager, *store.ChainStore, *stmgr.StateManager, EventHelperAPI, *messagepool.MessagePool, full.StateAPI, full.ChainAPI) (*full.EthEventHandler, error) {
	return func(mctx helpers.MetricsCtx, r repo.LockedRepo, lc fx.Lifecycle, fm *filter.EventFilterManager, cs *store.ChainStore, sm *stmgr.StateManager, evapi EventHelperAPI, mp *messagepool.MessagePool, stateapi full.StateAPI, chainapi full.ChainAPI) (*full.EthEventHandler, error) {
		ctx := helpers.LifecycleCtx(mctx, lc)

		ee := &full.EthEventHandler{
			Chain:                cs,
			MaxFilterHeightRange: abi.ChainEpoch(cfg.Events.MaxFilterHeightRange),
			SubscribtionCtx:      ctx,
		}

		if !cfg.EnableEthRPC || cfg.Events.DisableRealTimeFilterAPI {
			// all event functionality is disabled
			// the historic filter API relies on the real time one
			return ee, nil
		}

		ee.SubManager = &full.EthSubscriptionManager{
			Chain:    cs,
			StateAPI: stateapi,
			ChainAPI: chainapi,
		}
		ee.FilterStore = filter.NewMemFilterStore(cfg.Events.MaxFilters)

		// Start garbage collection for filters
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				go ee.GC(ctx, time.Duration(cfg.Events.FilterTTL))
				return nil
			},
		})

		ee.TipSetFilterManager = &filter.TipSetFilterManager{
			MaxFilterResults: cfg.Events.MaxFilterResults,
		}
		ee.MemPoolFilterManager = &filter.MemPoolFilterManager{
			MaxFilterResults: cfg.Events.MaxFilterResults,
		}
		ee.EventFilterManager = fm

		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				ev, err := events.NewEvents(ctx, &evapi)
				if err != nil {
					return err
				}
				// ignore returned tipsets
				_ = ev.Observe(ee.TipSetFilterManager)

				ch, err := mp.Updates(ctx)
				if err != nil {
					return err
				}
				go ee.MemPoolFilterManager.WaitForMpoolUpdates(ctx, ch)

				return nil
			},
		})

		return ee, nil
	}
}

func EventFilterManager(cfg config.FevmConfig) func(helpers.MetricsCtx, repo.LockedRepo, fx.Lifecycle, *store.ChainStore, *stmgr.StateManager, EventHelperAPI, full.ChainAPI) (*filter.EventFilterManager, error) {
	return func(mctx helpers.MetricsCtx, r repo.LockedRepo, lc fx.Lifecycle, cs *store.ChainStore, sm *stmgr.StateManager, evapi EventHelperAPI, chainapi full.ChainAPI) (*filter.EventFilterManager, error) {
		ctx := helpers.LifecycleCtx(mctx, lc)

		// Enable indexing of actor events
		var eventIndex *filter.EventIndex
		if !cfg.Events.DisableHistoricFilterAPI {
			var dbPath string
			if cfg.Events.DatabasePath == "" {
				sqlitePath, err := r.SqlitePath()
				if err != nil {
					return nil, err
				}
				dbPath = filepath.Join(sqlitePath, "events.db")
			} else {
				dbPath = cfg.Events.DatabasePath
			}

			var err error
			eventIndex, err = filter.NewEventIndex(ctx, dbPath, chainapi.Chain)
			if err != nil {
				return nil, err
			}

			lc.Append(fx.Hook{
				OnStop: func(context.Context) error {
					return eventIndex.Close()
				},
			})
		}

		fm := &filter.EventFilterManager{
			ChainStore: cs,
			EventIndex: eventIndex, // will be nil unless EnableHistoricFilterAPI is true
			// TODO:
			// We don't need this address resolution anymore once https://github.com/filecoin-project/lotus/issues/11594 lands
			AddressResolver: func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
				idAddr, err := address.NewIDAddress(uint64(emitter))
				if err != nil {
					return address.Undef, false
				}

				actor, err := sm.LoadActor(ctx, idAddr, ts)
				if err != nil || actor.Address == nil {
					return idAddr, true
				}

				return *actor.Address, true
			},

			MaxFilterResults: cfg.Events.MaxFilterResults,
		}

		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				ev, err := events.NewEvents(ctx, &evapi)
				if err != nil {
					return err
				}
				_ = ev.Observe(fm)
				return nil
			},
		})

		return fm, nil
	}
}

func ActorEventHandler(enable bool, fevmCfg config.FevmConfig) func(helpers.MetricsCtx, repo.LockedRepo, fx.Lifecycle, *filter.EventFilterManager, *store.ChainStore, *stmgr.StateManager, EventHelperAPI, *messagepool.MessagePool, full.StateAPI, full.ChainAPI) (*full.ActorEventHandler, error) {
	return func(mctx helpers.MetricsCtx, r repo.LockedRepo, lc fx.Lifecycle, fm *filter.EventFilterManager, cs *store.ChainStore, sm *stmgr.StateManager, evapi EventHelperAPI, mp *messagepool.MessagePool, stateapi full.StateAPI, chainapi full.ChainAPI) (*full.ActorEventHandler, error) {

		if !enable || fevmCfg.Events.DisableRealTimeFilterAPI {
			fm = nil
		}

		return full.NewActorEventHandler(
			cs,
			fm,
			time.Duration(build.BlockDelaySecs)*time.Second,
			abi.ChainEpoch(fevmCfg.Events.MaxFilterHeightRange),
		), nil
	}
}
