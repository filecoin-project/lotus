package modules

import (
	"context"
	"time"

	"github.com/multiformats/go-varint"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
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

func EthEventAPI(cfg config.ActorEventConfig) func(helpers.MetricsCtx, fx.Lifecycle, *store.ChainStore, *stmgr.StateManager, EventAPI, *messagepool.MessagePool, full.StateAPI, full.ChainAPI) (*full.EthEvent, error) {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, cs *store.ChainStore, sm *stmgr.StateManager, evapi EventAPI, mp *messagepool.MessagePool, stateapi full.StateAPI, chainapi full.ChainAPI) (*full.EthEvent, error) {
		ctx := helpers.LifecycleCtx(mctx, lc)

		ee := &full.EthEvent{
			Chain:                cs,
			MaxFilterHeightRange: abi.ChainEpoch(cfg.MaxFilterHeightRange),
		}

		if !cfg.EnableRealTimeFilterAPI {
			// all event functionality is disabled
			// the historic filter API relies on the real time one
			return ee, nil
		}

		ee.SubManager = &full.EthSubscriptionManager{
			Chain:    cs,
			StateAPI: stateapi,
			ChainAPI: chainapi,
		}
		ee.FilterStore = filter.NewMemFilterStore(cfg.MaxFilters)

		// Start garbage collection for filters
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				go ee.GC(ctx, time.Duration(cfg.FilterTTL))
				return nil
			},
		})

		// Enable indexing of actor events
		var eventIndex *filter.EventIndex
		if cfg.EnableHistoricFilterAPI {
			var err error
			eventIndex, err = filter.NewEventIndex(cfg.ActorEventDatabasePath)
			if err != nil {
				return nil, err
			}

			lc.Append(fx.Hook{
				OnStop: func(context.Context) error {
					return eventIndex.Close()
				},
			})
		}

		ee.EventFilterManager = &filter.EventFilterManager{
			ChainStore: cs,
			EventIndex: eventIndex, // will be nil unless EnableHistoricFilterAPI is true
			AddressResolver: func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
				// we only want to match using f4 addresses
				idAddr, err := address.NewIDAddress(uint64(emitter))
				if err != nil {
					return address.Undef, false
				}

				actor, err := sm.LoadActor(ctx, idAddr, ts)
				if err != nil || actor.Address == nil {
					return address.Undef, false
				}

				// if robust address is not f4 then we won't match against it so bail early
				if actor.Address.Protocol() != address.Delegated {
					return address.Undef, false
				}
				// we have an f4 address, make sure it's assigned by the EAM
				if namespace, _, err := varint.FromUvarint(actor.Address.Payload()); err != nil || namespace != builtintypes.EthereumAddressManagerActorID {
					return address.Undef, false
				}
				return *actor.Address, true
			},

			MaxFilterResults: cfg.MaxFilterResults,
		}
		ee.TipSetFilterManager = &filter.TipSetFilterManager{
			MaxFilterResults: cfg.MaxFilterResults,
		}
		ee.MemPoolFilterManager = &filter.MemPoolFilterManager{
			MaxFilterResults: cfg.MaxFilterResults,
		}

		const ChainHeadConfidence = 1

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

		return ee, nil
	}
}
