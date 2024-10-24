package modules

import (
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func StateManager(lc fx.Lifecycle, cs *store.ChainStore, exec stmgr.Executor, sys vm.SyscallBuilder, us stmgr.UpgradeSchedule, b beacon.Schedule, metadataDs dtypes.MetadataDS, chainIndexer index.Indexer) (*stmgr.StateManager, error) {
	sm, err := stmgr.NewStateManager(cs, exec, sys, us, b, metadataDs, chainIndexer)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStart: sm.Start,
		OnStop:  sm.Stop,
	})
	return sm, nil
}
