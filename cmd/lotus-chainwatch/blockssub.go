package main

import (
	"context"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"

	aapi "github.com/filecoin-project/lotus/api"
)

func subBlocks(ctx context.Context, api aapi.FullNode, st *storage) {
	sub, err := api.SyncIncomingBlocks(ctx)
	if err != nil {
		log.Error(err)
		return
	}

	for bh := range sub {
		err := st.storeHeaders(map[cid.Cid]*types.BlockHeader{
			bh.Cid(): bh,
		}, false)
		if err != nil {
			//log.Errorf("%+v", err)
		}
	}
}
