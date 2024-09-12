package modules

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/golang-lru/arc/v2"
	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/ethhashlookup"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

func EthTxHashManager(cfg config.FevmConfig) func(helpers.MetricsCtx, repo.LockedRepo, fx.Lifecycle, *store.ChainStore, EventHelperAPI, *messagepool.MessagePool, full.StateAPI, full.SyncAPI) (full.EthTxHashManager, error) {
	return func(mctx helpers.MetricsCtx, r repo.LockedRepo, lc fx.Lifecycle, cs *store.ChainStore, evapi EventHelperAPI, mp *messagepool.MessagePool, stateapi full.StateAPI, syncapi full.SyncAPI) (full.EthTxHashManager, error) {
		ctx := helpers.LifecycleCtx(mctx, lc)

		sqlitePath, err := r.SqlitePath()
		if err != nil {
			return nil, err
		}

		dbPath := filepath.Join(sqlitePath, ethhashlookup.DefaultDbFilename)

		// Check if the db exists, if not, we'll back-fill some entries
		_, err = os.Stat(dbPath)
		dbAlreadyExists := err == nil

		transactionHashLookup, err := ethhashlookup.NewTransactionHashLookup(ctx, dbPath)
		if err != nil {
			return nil, err
		}

		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				return transactionHashLookup.Close()
			},
		})

		ethTxHashManager := full.NewEthTxHashManager(stateapi, transactionHashLookup)

		if !dbAlreadyExists {
			err = ethTxHashManager.PopulateExistingMappings(mctx, 0)
			if err != nil {
				return nil, err
			}
		}

		// prefill the whole skiplist cache maintained internally by the GetTipsetByHeight
		go func() {
			start := time.Now()
			log.Infoln("Start prefilling GetTipsetByHeight cache")
			_, err := cs.GetTipsetByHeight(mctx, abi.ChainEpoch(0), cs.GetHeaviestTipSet(), false)
			if err != nil {
				log.Warnf("error when prefilling GetTipsetByHeight cache: %w", err)
			}
			log.Infof("Prefilling GetTipsetByHeight done in %s", time.Since(start))
		}()

		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				ev, err := events.NewEvents(ctx, &evapi)
				if err != nil {
					return err
				}

				// Tipset listener
				_ = ev.Observe(ethTxHashManager)

				ch, err := mp.Updates(ctx)
				if err != nil {
					return err
				}
				go full.WaitForMpoolUpdates(ctx, ch, ethTxHashManager)
				go full.EthTxHashGC(ctx, cfg.EthTxHashMappingLifetimeDays, ethTxHashManager)

				return nil
			},
		})

		return ethTxHashManager, nil
	}
}

func EthModuleAPI(cfg config.FevmConfig) func(helpers.MetricsCtx, repo.LockedRepo, fx.Lifecycle, *store.ChainStore, *stmgr.StateManager, *messagepool.MessagePool, full.StateAPI, full.ChainAPI, full.MpoolAPI, full.SyncAPI, *full.EthEventHandler, full.EthTxHashManager) (*full.EthModule, error) {
	return func(mctx helpers.MetricsCtx, r repo.LockedRepo, lc fx.Lifecycle, cs *store.ChainStore, sm *stmgr.StateManager, mp *messagepool.MessagePool, stateapi full.StateAPI, chainapi full.ChainAPI, mpoolapi full.MpoolAPI, syncapi full.SyncAPI, ethEventHandler *full.EthEventHandler, ethTxHashManager full.EthTxHashManager) (*full.EthModule, error) {

		var blkCache *arc.ARCCache[cid.Cid, *ethtypes.EthBlock]
		var blkTxCache *arc.ARCCache[cid.Cid, *ethtypes.EthBlock]
		var err error
		if cfg.EthBlkCacheSize > 0 {
			blkCache, err = arc.NewARC[cid.Cid, *ethtypes.EthBlock](cfg.EthBlkCacheSize)
			if err != nil {
				return nil, xerrors.Errorf("failed to create block cache: %w", err)
			}

			blkTxCache, err = arc.NewARC[cid.Cid, *ethtypes.EthBlock](cfg.EthBlkCacheSize)
			if err != nil {
				return nil, xerrors.Errorf("failed to create block transaction cache: %w", err)
			}
		}

		return &full.EthModule{
			Chain:        cs,
			Mpool:        mp,
			StateManager: sm,

			ChainAPI:        chainapi,
			MpoolAPI:        mpoolapi,
			StateAPI:        stateapi,
			SyncAPI:         syncapi,
			EthEventHandler: ethEventHandler,

			EthTxHashManager:         ethTxHashManager,
			EthTraceFilterMaxResults: cfg.EthTraceFilterMaxResults,

			EthBlkCache:   blkCache,
			EthBlkTxCache: blkTxCache,
		}, nil
	}
}
