package modules

import (
	"go.uber.org/fx"

	"github.com/brossetti1/lotus/chain/beacon"
	"github.com/brossetti1/lotus/chain/stmgr"
	"github.com/brossetti1/lotus/chain/store"
	"github.com/brossetti1/lotus/chain/vm"
	"github.com/brossetti1/lotus/node/modules/dtypes"
)

func StateManager(lc fx.Lifecycle, cs *store.ChainStore, exec stmgr.Executor, sys vm.SyscallBuilder, us stmgr.UpgradeSchedule, b beacon.Schedule, metadataDs dtypes.MetadataDS) (*stmgr.StateManager, error) {
	sm, err := stmgr.NewStateManager(cs, exec, sys, us, b, metadataDs)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStart: sm.Start,
		OnStop:  sm.Stop,
	})
	return sm, nil
}
