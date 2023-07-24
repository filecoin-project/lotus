package modules

import (
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/impl/full"
)

func EthTraceAPI() func(*store.ChainStore, *stmgr.StateManager, full.EthModuleAPI, full.ChainAPI) (*full.EthTrace, error) {
	return func(cs *store.ChainStore, sm *stmgr.StateManager, evapi full.EthModuleAPI, chainapi full.ChainAPI) (*full.EthTrace, error) {
		return &full.EthTrace{
			Chain:        cs,
			StateManager: sm,

			ChainAPI:     chainapi,
			EthModuleAPI: evapi,
		}, nil
	}
}
