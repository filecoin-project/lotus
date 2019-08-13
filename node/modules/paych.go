package modules

import (
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/filecoin-project/go-lotus/paych"
)

func PaychStore(ds dtypes.MetadataDS) *paych.Store {
	return paych.NewStore(ds)
}

func PaymentChannelManager(chain *store.ChainStore, store *paych.Store) (*paych.Manager, error) {
	return paych.NewManager(chain, store), nil
}
