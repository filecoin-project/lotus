package modules

import (
	"context"
	"os"
	"time"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

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
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type TipSetResolverParams struct {
	fx.In
	ChainStore eth.ChainStore
	F3         full.F3ModuleAPI `optional:"true"`
}

func MakeV1TipSetResolver(params TipSetResolverParams) full.EthTipSetResolverV2 {
	// TODO: remove this env var in a future release and remove all special-casing for Eth v1 in
	// builder_chain.go with a single path to instantiating Eth modules and re-using them for
	// both v1 and v2 APIs.
	// The env var is only intended as a temporary escape hatch for users who encounter issues
	// with F3 certificate–based finality resolution in Eth APIs.
	f3CertificateProvider := params.F3
	useF3ForFinality := true
	if os.Getenv("LOTUS_ETH_V1_DISABLE_F3_FINALITY_RESOLUTION") == "1" {
		f3CertificateProvider = nil
		useF3ForFinality = false
	}
	return eth.NewTipSetResolver(params.ChainStore, f3CertificateProvider, useF3ForFinality)
}

func MakeV2TipSetResolver(params TipSetResolverParams) full.EthTipSetResolverV2 {
	return eth.NewTipSetResolver(params.ChainStore, params.F3, true)
}

func MakeEthFilecoinV1(stateManager eth.StateManager, tipsetResolver full.EthTipSetResolverV1) full.EthFilecoinAPIV1 {
	return eth.NewEthFilecoinAPI(stateManager, tipsetResolver)
}

func MakeEthFilecoinV2(stateManager eth.StateManager, tipsetResolver full.EthTipSetResolverV2) full.EthFilecoinAPIV1 {
	return eth.NewEthFilecoinAPI(stateManager, tipsetResolver)
}

func MakeEthLookupV1(
	chainStore eth.ChainStore,
	stateManager eth.StateManager,
	syncApi eth.SyncAPI,
	stateBlockstore dtypes.StateBlockstore,
	tipsetResolver full.EthTipSetResolverV1,
) full.EthLookupAPIV1 {
	return eth.NewEthLookupAPI(chainStore, stateManager, syncApi, stateBlockstore, tipsetResolver)
}

func MakeEthLookupV2(
	chainStore eth.ChainStore,
	stateManager eth.StateManager,
	syncApi eth.SyncAPI,
	stateBlockstore dtypes.StateBlockstore,
	tipsetResolver full.EthTipSetResolverV2,
) full.EthLookupAPIV2 {
	return eth.NewEthLookupAPI(chainStore, stateManager, syncApi, stateBlockstore, tipsetResolver)
}

func MakeEthGasV1(
	chainStore eth.ChainStore,
	stateManager eth.StateManager,
	messagePool eth.MessagePool,
	gasApi eth.GasAPI,
	tipsetResolver full.EthTipSetResolverV1,
) full.EthGasAPIV1 {
	return eth.NewEthGasAPI(chainStore, stateManager, messagePool, gasApi, tipsetResolver)
}

func MakeEthGasV2(
	chainStore eth.ChainStore,
	stateManager eth.StateManager,
	messagePool eth.MessagePool,
	gasApi eth.GasAPI,
	tipsetResolver full.EthTipSetResolverV2,
) full.EthGasAPIV2 {
	return eth.NewEthGasAPI(chainStore, stateManager, messagePool, gasApi, tipsetResolver)
}

type EthTransactionParams struct {
	fx.In

	MetricsCtx        helpers.MetricsCtx
	Lifecycle         fx.Lifecycle
	ChainStore        eth.ChainStore
	StateManager      eth.StateManager
	StateAPI          eth.StateAPI
	MpoolAPI          eth.MpoolAPI
	EthEventsExtended eth.EthEventsInternal
	Indexer           index.Indexer
}

func MakeEthTransactionV1(cfg config.FevmConfig) func(EthTransactionParams, full.EthTipSetResolverV1) (full.EthTransactionAPIV1, error) {
	return func(params EthTransactionParams, tipSetResolver full.EthTipSetResolverV1) (full.EthTransactionAPIV1, error) {
		return makeEthTransaction(params, tipSetResolver, cfg.EthBlkCacheSize)
	}
}

func MakeEthTransactionV2(cfg config.FevmConfig) func(EthTransactionParams, full.EthTipSetResolverV2) (full.EthTransactionAPIV2, error) {
	return func(params EthTransactionParams, tipSetResolver full.EthTipSetResolverV2) (full.EthTransactionAPIV2, error) {
		return makeEthTransaction(params, tipSetResolver, cfg.EthBlkCacheSize)
	}
}

func makeEthTransaction(params EthTransactionParams, tipSetResolver eth.TipSetResolver, ethBlkCacheSize int) (full.EthTransactionAPIV1, error) {
	// Prime the tipset cache with the entire chain to make sure tx and block lookups are fast
	ctx := helpers.LifecycleCtx(params.MetricsCtx, params.Lifecycle) // cancelled OnStop
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go func() {
				start := time.Now()
				log.Infoln("Start prefilling GetTipsetByHeight cache")
				_, err := params.ChainStore.GetTipsetByHeight(ctx, abi.ChainEpoch(0), params.ChainStore.GetHeaviestTipSet(), false)
				if err != nil {
					log.Warnf("error when prefilling GetTipsetByHeight cache: %w", err)
				}
				log.Infof("Prefilling GetTipsetByHeight done in %s", time.Since(start))
			}()
			return nil
		},
	})

	return eth.NewEthTransactionAPI(
		params.ChainStore,
		params.StateManager,
		params.StateAPI,
		params.MpoolAPI,
		params.Indexer,
		params.EthEventsExtended,
		tipSetResolver,
		ethBlkCacheSize,
	)
}

func MakeEthTraceV1(cfg config.FevmConfig) func(
	chainStore eth.ChainStore,
	stateManager eth.StateManager,
	ethTransaction full.EthTransactionAPIV1,
	tipsetResolver full.EthTipSetResolverV1,
) full.EthTraceAPIV1 {
	return func(
		chainStore eth.ChainStore,
		stateManager eth.StateManager,
		ethTransaction full.EthTransactionAPIV1,
		tipsetResolver full.EthTipSetResolverV1,
	) full.EthTraceAPIV1 {
		return eth.NewEthTraceAPI(chainStore, stateManager, ethTransaction, tipsetResolver, cfg.EthTraceFilterMaxResults)
	}
}

func MakeEthTraceV2(cfg config.FevmConfig) func(
	chainStore eth.ChainStore,
	stateManager eth.StateManager,
	ethTransaction full.EthTransactionAPIV2,
	tipsetResolver full.EthTipSetResolverV2,
) full.EthTraceAPIV2 {
	return func(
		chainStore eth.ChainStore,
		stateManager eth.StateManager,
		ethTransaction full.EthTransactionAPIV2,
		tipsetResolver full.EthTipSetResolverV2,
	) full.EthTraceAPIV2 {
		return eth.NewEthTraceAPI(chainStore, stateManager, ethTransaction, tipsetResolver, cfg.EthTraceFilterMaxResults)
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

	MetricsCtx         helpers.MetricsCtx
	Lifecycle          fx.Lifecycle
	EventFilterManager *filter.EventFilterManager
	ChainStore         *store.ChainStore
	StateManager       *stmgr.StateManager
	EventHelperAPI     EventHelperAPI
	MessagePool        *messagepool.MessagePool
	Indexer            index.Indexer
}

func MakeEthEventsExtended(cfg config.EventsConfig, enableEthRPC bool) func(EthEventsParams) (eth.EthEventsInternal, error) {
	return func(params EthEventsParams) (eth.EthEventsInternal, error) {
		lctx := helpers.LifecycleCtx(params.MetricsCtx, params.Lifecycle)

		var (
			subscriptionCtx                       = lctx
			chainStore           eth.ChainStore   = params.ChainStore
			stateManager         eth.StateManager = params.StateManager
			chainIndexer         index.Indexer    = params.Indexer
			eventFilterManager   *filter.EventFilterManager
			tipSetFilterManager  *filter.TipSetFilterManager
			memPoolFilterManager *filter.MemPoolFilterManager
			filterStore          filter.FilterStore
			subscriptionManager  *eth.EthSubscriptionManager
			maxFilterHeightRange = abi.ChainEpoch(cfg.MaxFilterHeightRange)
		)

		if !enableEthRPC {
			// all event functionality is disabled
			// the historic filter API relies on the real time one
			return eth.NewEthEventsAPI(
				subscriptionCtx,
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

		ee := eth.NewEthEventsAPI(
			subscriptionCtx,
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
				ev, err := events.NewEvents(lctx, &params.EventHelperAPI)
				if err != nil {
					return err
				}
				// ignore returned tipsets
				_ = ev.Observe(tipSetFilterManager)

				ch, err := params.MessagePool.Updates(lctx)
				if err != nil {
					return err
				}
				go memPoolFilterManager.WaitForMpoolUpdates(lctx, ch)

				go ee.GC(lctx, time.Duration(cfg.FilterTTL))

				return nil
			},
		})

		return ee, nil
	}
}

// GatewayEthSend is a helper to provide the Gateway with the EthSendAPI but block the use of
// EthSendRawTransactionUntrusted. The Gateway API doesn't expose this method, so this is a
// precautionary measure.
type GatewayEthSend struct {
	fx.In
	eth.EthSendAPI
}

func (*GatewayEthSend) EthSendRawTransactionUntrusted(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	return ethtypes.EthHash{}, xerrors.New("EthSendRawTransactionUntrusted is not supported in gateway mode")
}

// GatewayEthTransaction is a helper to provide the Gateway with the EthTransactionAPI but block the
// use of EthGetTransactionByHashLimited, EthGetTransactionReceiptLimited and
// EthGetBlockReceiptsLimited. The Gateway API doesn't expose these methods, so this is a
// precautionary measure.
type GatewayEthTransaction struct {
	fx.In
	eth.EthTransactionAPI
}

func (*GatewayEthTransaction) EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error) {
	return nil, xerrors.New("EthGetTransactionByHashLimited is not supported in gateway mode")
}
func (*GatewayEthTransaction) EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTxReceipt, error) {
	return nil, xerrors.New("EthGetTransactionByHashLimited is not supported in gateway mode")
}
func (*GatewayEthTransaction) EthGetBlockReceiptsLimited(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, limit abi.ChainEpoch) ([]*ethtypes.EthTxReceipt, error) {
	return nil, xerrors.New("EthGetTransactionByHashLimited is not supported in gateway mode")
}

type EventFilterManagerParams struct {
	fx.In

	MetricsCtx     helpers.MetricsCtx
	Lifecycle      fx.Lifecycle
	ChainStore     *store.ChainStore
	StateManager   *stmgr.StateManager
	EventHelperAPI EventHelperAPI
	Indexer        index.Indexer
}

func MakeEventFilterManager(cfg config.EventsConfig) func(EventFilterManagerParams) (*filter.EventFilterManager, error) {
	return func(params EventFilterManagerParams) (*filter.EventFilterManager, error) {
		lctx := helpers.LifecycleCtx(params.MetricsCtx, params.Lifecycle)

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
				ev, err := events.NewEvents(lctx, &params.EventHelperAPI)
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
