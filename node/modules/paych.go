package modules

import (
	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/filecoin-project/go-lotus/paych"
)

func PaychStore(ds dtypes.MetadataDS) *paych.Store {
	return paych.NewStore(ds)
}

func PaymentChannelManager(chain *store.ChainStore, store *paych.Store) (*paych.Manager, error) {
	var api api.FullNode
	panic("i need a full node api. what do i do")
	return paych.NewManager(api, chain, store), nil
}
