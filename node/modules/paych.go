package modules

import (
	"github.com/filecoin-project/go-lotus/chain/stmgr"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/filecoin-project/go-lotus/paych"
)

func PaychStore(ds dtypes.MetadataDS) *paych.Store {
	return paych.NewStore(ds)
}

func PaymentChannelManager(sm *stmgr.StateManager, store *paych.Store) (*paych.Manager, error) {
	return paych.NewManager(sm, store), nil
}
