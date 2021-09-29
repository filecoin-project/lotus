package modules

import (
	"github.com/filecoin-project/lotus/chain/vm"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
)

func StateManager(lc fx.Lifecycle, cs *store.ChainStore, exec stmgr.Executor, sys vm.SyscallBuilder, us stmgr.UpgradeSchedule) (*stmgr.StateManager, error) {
	sm, err := stmgr.NewStateManager(cs, exec, sys, us)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStart: sm.Start,
		OnStop:  sm.Stop,
	})
	return sm, nil
}
