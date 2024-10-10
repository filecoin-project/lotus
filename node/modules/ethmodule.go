package modules

import (
	"time"

	"github.com/hashicorp/golang-lru/arc/v2"
	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

func EthModuleAPI(cfg config.FevmConfig) func(helpers.MetricsCtx, repo.LockedRepo, fx.Lifecycle, *store.ChainStore, *stmgr.StateManager,
	EventHelperAPI, *messagepool.MessagePool, full.StateAPI, full.ChainAPI, full.MpoolAPI, full.SyncAPI, *full.EthEventHandler, index.Indexer) (*full.EthModule, error) {
	return func(mctx helpers.MetricsCtx, r repo.LockedRepo, lc fx.Lifecycle, cs *store.ChainStore, sm *stmgr.StateManager, evapi EventHelperAPI,
		mp *messagepool.MessagePool, stateapi full.StateAPI, chainapi full.ChainAPI, mpoolapi full.MpoolAPI, syncapi full.SyncAPI,
		ethEventHandler *full.EthEventHandler, chainIndexer index.Indexer) (*full.EthModule, error) {

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

		var err error
		var blkCache *arc.ARCCache[cid.Cid, *ethtypes.EthBlock]
		var blkTxCache *arc.ARCCache[cid.Cid, *ethtypes.EthBlock]
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

			EthTraceFilterMaxResults: cfg.EthTraceFilterMaxResults,

			EthBlkCache:   blkCache,
			EthBlkTxCache: blkTxCache,

			ChainIndexer: chainIndexer,
		}, nil
	}
}
