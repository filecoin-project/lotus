package modules

import (
	"context"
	"path/filepath"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

func ChainIndexer(cfg config.ChainIndexerConfig) func(lc fx.Lifecycle, mctx helpers.MetricsCtx, cs *store.ChainStore, r repo.LockedRepo) (index.Indexer, error) {
	return func(lc fx.Lifecycle, mctx helpers.MetricsCtx, cs *store.ChainStore, r repo.LockedRepo) (index.Indexer, error) {
		if !cfg.EnableIndexer {
			log.Infof("ChainIndexer is disabled")
			return nil, nil
		}

		chainIndexPath, err := r.ChainIndexPath()
		if err != nil {
			return nil, err
		}

		dbPath := filepath.Join(chainIndexPath, index.DefaultDbFilename)
		chainIndexer, err := index.NewSqliteIndexer(dbPath, cs, cfg.GCRetentionEpochs, cfg.ReconcileEmptyIndex, cfg.MaxReconcileTipsets)
		if err != nil {
			return nil, err
		}

		lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				return chainIndexer.Close()
			},
		})

		return chainIndexer, nil
	}
}

func InitChainIndexer(cfg config.ChainIndexerConfig) func(lc fx.Lifecycle, mctx helpers.MetricsCtx, indexer index.Indexer,
	evapi EventHelperAPI, mp *messagepool.MessagePool, sm *stmgr.StateManager) {
	return func(lc fx.Lifecycle, mctx helpers.MetricsCtx, indexer index.Indexer,
		evapi EventHelperAPI, mp *messagepool.MessagePool, sm *stmgr.StateManager) {
		ctx := helpers.LifecycleCtx(mctx, lc)

		lc.Append(fx.Hook{
			OnStart: func(_ context.Context) error {
				indexer.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
					idAddr, err := address.NewIDAddress(uint64(emitter))
					if err != nil {
						return address.Undef, false
					}

					actor, err := sm.LoadActor(ctx, idAddr, ts)
					if err != nil || actor.DelegatedAddress == nil {
						return idAddr, true
					}

					return *actor.DelegatedAddress, true
				})

				indexer.SetRecomputeTipSetStateFunc(func(ctx context.Context, ts *types.TipSet) error {
					_, _, err := sm.RecomputeTipSetState(ctx, ts)
					return err
				})

				ev, err := events.NewEvents(ctx, &evapi)
				if err != nil {
					return err
				}

				// Tipset listener

				// `ObserveAndBlock` returns the current head and guarantees that it will call the observer with all future tipsets
				head, unlockObserver, err := ev.ObserveAndBlock(indexer)
				if err != nil {
					return xerrors.Errorf("error while observing tipsets: %w", err)
				}
				if err := indexer.ReconcileWithChain(ctx, head); err != nil {
					unlockObserver()
					if !cfg.AllowIndexReconciliationFailure {
						return xerrors.Errorf("error while reconciling chain index with chain state: %w", err)
					}
					log.Warnf("error while reconciling chain index with chain state: %s", err)
				} else {
					unlockObserver()
				}

				// Reconciliation holds the indexer's writer lock for its whole run, so
				// mpool messages are indexed from here on; earlier ones are indexed
				// when included in a tipset.
				ch, err := mp.Updates(ctx)
				if err != nil {
					return err
				}
				go WaitForMpoolUpdates(ctx, ch, indexer)

				indexer.Start()

				return nil
			},
		})
	}
}

func ChainIndexHandler(cfg config.ChainIndexerConfig) func(helpers.MetricsCtx, repo.LockedRepo, fx.Lifecycle, index.Indexer) (*full.ChainIndexHandler, error) {
	return func(mctx helpers.MetricsCtx, r repo.LockedRepo, lc fx.Lifecycle, indexer index.Indexer) (*full.ChainIndexHandler, error) {
		return full.NewChainIndexHandler(indexer), nil
	}
}

const mpoolIndexQueueSize = 1024

// WaitForMpoolUpdates indexes delegated messages added to the mpool under
// their eth tx hash. The mpool publishes updates while holding its lock, so
// this loop only queues them; a second goroutine indexes, waiting on the
// indexer's writer lock as needed. Messages that overflow the queue are
// indexed when included in a tipset.
func WaitForMpoolUpdates(ctx context.Context, ch <-chan api.MpoolUpdate, indexer index.Indexer) {
	queue := make(chan *types.SignedMessage, mpoolIndexQueueSize)
	defer close(queue)
	go func() {
		for msg := range queue {
			if ctx.Err() != nil {
				return
			}
			if err := indexer.IndexSignedMessage(ctx, msg); err != nil {
				log.Errorw("failed to index signed Mpool message", "error", err)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case u, ok := <-ch:
			if !ok {
				return
			}
			if u.Type != api.MpoolAdd || u.Message == nil || u.Message.Signature.Type != crypto.SigTypeDelegated {
				continue
			}
			select {
			case queue <- u.Message:
			default:
				log.Warnw("mpool eth tx hash index queue full; message will be indexed on inclusion", "cid", u.Message.Cid())
			}
		}
	}
}
