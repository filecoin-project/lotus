package modules

import (
	"context"
	"os"
	"path/filepath"

	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/chain/ethhashlookup"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

func EthModuleAPI(cfg config.FevmConfig) func(helpers.MetricsCtx, repo.LockedRepo, fx.Lifecycle, *store.ChainStore, *stmgr.StateManager, EventAPI, *messagepool.MessagePool, full.StateAPI, full.ChainAPI, full.MpoolAPI) (*full.EthModule, error) {
	return func(mctx helpers.MetricsCtx, r repo.LockedRepo, lc fx.Lifecycle, cs *store.ChainStore, sm *stmgr.StateManager, evapi EventAPI, mp *messagepool.MessagePool, stateapi full.StateAPI, chainapi full.ChainAPI, mpoolapi full.MpoolAPI) (*full.EthModule, error) {
		sqlitePath, err := r.SqlitePath()
		if err != nil {
			return nil, err
		}

		dbPath := filepath.Join(sqlitePath, "txhash.db")

		// Check if the db exists, if not, we'll back-fill some entries
		_, err = os.Stat(dbPath)
		dbAlreadyExists := err == nil

		transactionHashLookup, err := ethhashlookup.NewTransactionHashLookup(dbPath)
		if err != nil {
			return nil, err
		}

		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				return transactionHashLookup.Close()
			},
		})

		ethTxHashManager := full.EthTxHashManager{
			StateAPI:              stateapi,
			TransactionHashLookup: transactionHashLookup,
		}

		if !dbAlreadyExists {
			err = ethTxHashManager.PopulateExistingMappings(mctx, 0)
			if err != nil {
				return nil, err
			}
		}

		const ChainHeadConfidence = 1

		ctx := helpers.LifecycleCtx(mctx, lc)
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				ev, err := events.NewEventsWithConfidence(ctx, &evapi, ChainHeadConfidence)
				if err != nil {
					return err
				}

				// Tipset listener
				_ = ev.Observe(&ethTxHashManager)

				ch, err := mp.Updates(ctx)
				if err != nil {
					return err
				}
				go full.WaitForMpoolUpdates(ctx, ch, &ethTxHashManager)
				go full.EthTxHashGC(ctx, cfg.EthTxHashMappingLifetimeDays, &ethTxHashManager)

				return nil
			},
		})

		return &full.EthModule{
			Chain:        cs,
			Mpool:        mp,
			StateManager: sm,

			ChainAPI: chainapi,
			MpoolAPI: mpoolapi,
			StateAPI: stateapi,

			EthTxHashManager: &ethTxHashManager,
		}, nil
	}
}
