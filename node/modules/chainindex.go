package modules

import (
	"context"
	"path/filepath"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chainindex"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

func ChainIndexer(cfg config.IndexConfig) func(lc fx.Lifecycle, mctx helpers.MetricsCtx, cs *store.ChainStore, r repo.LockedRepo) (chainindex.Indexer, error) {
	return func(lc fx.Lifecycle, mctx helpers.MetricsCtx, cs *store.ChainStore, r repo.LockedRepo) (chainindex.Indexer, error) {
		sqlitePath, err := r.SqlitePath()
		if err != nil {
			return nil, err
		}

		// TODO Implement config driven auto-backfilling
		chainIndexer, err := chainindex.NewSqliteIndexer(filepath.Join(sqlitePath, chainindex.DefaultDbFilename), cs, cfg.GCRetentionEpochs)
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

func InitChainIndexer(lc fx.Lifecycle, mctx helpers.MetricsCtx, indexer chainindex.Indexer,
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

			ev, err := events.NewEvents(ctx, &evapi)
			if err != nil {
				return err
			}

			// Tipset listener
			tipset, unlockObserver := ev.ObserveAndBlock(indexer)
			if err := indexer.ReconcileWithChain(ctx, tipset); err != nil {
				return xerrors.Errorf("error while reconciling chain index with chain state: %w", err)
			}
			unlockObserver()

			ch, err := mp.Updates(ctx)
			if err != nil {
				return err
			}
			go chainindex.WaitForMpoolUpdates(ctx, ch, indexer)

			return nil
		},
	})
}
