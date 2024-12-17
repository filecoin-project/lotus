package modules

import (
	"context"
	"time"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/eth"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

type EthTransactionParams struct {
	fx.In

	helpers.MetricsCtx
	fx.Lifecycle
	eth.ChainStoreAPI
	eth.StateManagerAPI
	eth.StateAPI
	eth.MpoolAPI
	eth.EthEventsExtended
	index.Indexer
}

func MakeEthTransaction(cfg config.FevmConfig) func(EthTransactionParams) (eth.EthTransaction, error) {
	return func(params EthTransactionParams) (eth.EthTransaction, error) {
		// Prime the tipset cache with the entire chain to make sure tx and block lookups are fast
		params.Lifecycle.Append(fx.Hook{
			OnStart: func(context.Context) error {
				go func() {
					start := time.Now()
					log.Infoln("Start prefilling GetTipsetByHeight cache")
					_, err := params.ChainStoreAPI.GetTipsetByHeight(params.MetricsCtx, abi.ChainEpoch(0), params.ChainStoreAPI.GetHeaviestTipSet(), false)
					if err != nil {
						log.Warnf("error when prefilling GetTipsetByHeight cache: %w", err)
					}
					log.Infof("Prefilling GetTipsetByHeight done in %s", time.Since(start))
				}()
				return nil
			},
		})

		return eth.NewEthTransaction(
			params.ChainStoreAPI,
			params.StateManagerAPI,
			params.StateAPI,
			params.MpoolAPI,
			params.Indexer,
			params.EthEventsExtended,
			cfg.EthBlkCacheSize,
		)
	}
}

func MakeEthTrace(cfg config.FevmConfig) func(
	chainStore eth.ChainStoreAPI,
	stateManager eth.StateManagerAPI,
	ethTransaction eth.EthTransaction,
) eth.EthTrace {
	return func(
		chainStore eth.ChainStoreAPI,
		stateManager eth.StateManagerAPI,
		ethTransaction eth.EthTransaction,
	) eth.EthTrace {
		return eth.NewEthTrace(chainStore, stateManager, ethTransaction, cfg.EthTraceFilterMaxResults)
	}
}

type EventHelperAPI struct {
	fx.In

	full.ChainAPI
	full.StateAPI
}

var _ events.EventHelperAPI = &EventHelperAPI{}

type EthEventsParams struct {
	fx.In

	helpers.MetricsCtx
	repo.LockedRepo
	fx.Lifecycle
	*filter.EventFilterManager
	*store.ChainStore
	*stmgr.StateManager
	EventHelperAPI
	*messagepool.MessagePool
	full.StateAPI
	full.ChainAPI
}

func MakeEthEventsExtended(cfg config.EventsConfig, enableEthRPC bool) func(EthEventsParams) (eth.EthEventsExtended, error) {
	return func(params EthEventsParams) (eth.EthEventsExtended, error) {
		ctx := helpers.LifecycleCtx(params.MetricsCtx, params.Lifecycle)

		var (
			subscribtionCtx      context.Context     = ctx
			chainStore           eth.ChainStoreAPI   = params.ChainStore
			stateManager         eth.StateManagerAPI = params.StateManager
			chainIndexer         index.Indexer       = params.ChainIndexer
			eventFilterManager   *filter.EventFilterManager
			tipSetFilterManager  *filter.TipSetFilterManager
			memPoolFilterManager *filter.MemPoolFilterManager
			filterStore          filter.FilterStore
			subscriptionManager  *eth.EthSubscriptionManager
			maxFilterHeightRange abi.ChainEpoch = abi.ChainEpoch(cfg.MaxFilterHeightRange)
		)

		if !enableEthRPC {
			// all event functionality is disabled
			// the historic filter API relies on the real time one
			return eth.NewEthEvents(
				subscribtionCtx,
				chainStore,
				stateManager,
				chainIndexer,
				eventFilterManager,
				tipSetFilterManager,
				memPoolFilterManager,
				filterStore,
				subscriptionManager,
				maxFilterHeightRange,
			), nil
		}

		subscriptionManager = eth.NewEthSubscriptionManager(chainStore, stateManager)
		filterStore = filter.NewMemFilterStore(cfg.MaxFilters)
		tipSetFilterManager = &filter.TipSetFilterManager{MaxFilterResults: cfg.MaxFilterResults}
		memPoolFilterManager = &filter.MemPoolFilterManager{MaxFilterResults: cfg.MaxFilterResults}
		eventFilterManager = params.EventFilterManager

		ee := eth.NewEthEvents(
			subscribtionCtx,
			chainStore,
			stateManager,
			chainIndexer,
			eventFilterManager,
			tipSetFilterManager,
			memPoolFilterManager,
			filterStore,
			subscriptionManager,
			maxFilterHeightRange,
		)

		params.Lifecycle.Append(fx.Hook{
			OnStart: func(context.Context) error {
				ev, err := events.NewEvents(ctx, &params.EventHelperAPI)
				if err != nil {
					return err
				}
				// ignore returned tipsets
				_ = ev.Observe(tipSetFilterManager)

				ch, err := params.MessagePool.Updates(ctx)
				if err != nil {
					return err
				}
				go memPoolFilterManager.WaitForMpoolUpdates(ctx, ch)

				go ee.GC(ctx, time.Duration(cfg.FilterTTL))

				return nil
			},
		})

		return ee, nil
	}
}

type GatewayEthSend struct {
	fx.In
	api.Gateway
}

func (*GatewayEthSend) EthSendRawTransactionUntrusted(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	return ethtypes.EthHash{}, xerrors.New("EthSendRawTransactionUntrusted is not supported in gateway mode")
}

type EventFilterManagerParams struct {
	fx.In

	helpers.MetricsCtx
	repo.LockedRepo
	fx.Lifecycle
	*store.ChainStore
	*stmgr.StateManager
	EventHelperAPI
	full.ChainAPI
	index.Indexer
}

func MakeEventFilterManager(cfg config.EventsConfig) func(EventFilterManagerParams) (*filter.EventFilterManager, error) {
	return func(params EventFilterManagerParams) (*filter.EventFilterManager, error) {
		ctx := helpers.LifecycleCtx(params.MetricsCtx, params.Lifecycle)

		// Enable indexing of actor events

		fm := &filter.EventFilterManager{
			ChainStore:   params.ChainStore,
			ChainIndexer: params.Indexer,
			AddressResolver: func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) address.Address {
				idAddr, err := address.NewIDAddress(uint64(emitter))
				if err != nil {
					return address.Undef
				}

				actor, err := params.StateManager.LoadActor(ctx, idAddr, ts)
				if err != nil || actor.DelegatedAddress == nil {
					return idAddr
				}

				return *actor.DelegatedAddress
			},

			MaxFilterResults: cfg.MaxFilterResults,
		}

		params.Lifecycle.Append(fx.Hook{
			OnStart: func(context.Context) error {
				ev, err := events.NewEvents(ctx, &params.EventHelperAPI)
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
