package modules

import (
	"context"
	"path/filepath"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

func ChainIndexer(cfg config.ChainIndexerConfig) func(lc fx.Lifecycle, mctx helpers.MetricsCtx, cs *store.ChainStore, r repo.LockedRepo) (index.Indexer, error) {
	return func(lc fx.Lifecycle, mctx helpers.MetricsCtx, cs *store.ChainStore, r repo.LockedRepo) (index.Indexer, error) {
		if !cfg.EnableIndexer {
			log.Infof("ChainIndexer is disabled")
			return nil, nil
		}

		sqlitePath, err := r.SqlitePath()
		if err != nil {
			return nil, err
		}

		// TODO Implement config driven auto-backfilling
		chainIndexer, err := index.NewSqliteIndexer(filepath.Join(sqlitePath, index.DefaultDbFilename),
			cs, cfg.GCRetentionEpochs, cfg.ReconcileEmptyIndex, cfg.MaxReconcileTipsets)
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

func InitChainIndexer(lc fx.Lifecycle, mctx helpers.MetricsCtx, indexer index.Indexer,
	evapi EventHelperAPI, mp *messagepool.MessagePool, sm *stmgr.StateManager) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			indexer.SetIdToRobustAddrFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
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

			ch, err := mp.Updates(ctx)
			if err != nil {
				return err
			}
			go index.WaitForMpoolUpdates(ctx, ch, indexer)

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
				return xerrors.Errorf("error while reconciling chain index with chain state: %w", err)
			}
			log.Infof("chain indexer reconciled with chain state; observer will start upates from height: %d", head.Height())
			unlockObserver()

			if err := indexer.Start(); err != nil {
				return err
			}

			return nil
		},
	})
}
