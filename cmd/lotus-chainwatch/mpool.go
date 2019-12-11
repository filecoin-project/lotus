package main

import (
	"context"

	"github.com/ipfs/go-cid"

	aapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

func subMpool(ctx context.Context, api aapi.FullNode, st *storage) {
	sub, err := api.MpoolSub(ctx)
	if err != nil {
		return
	}

	for change := range sub {
		if change.Type != aapi.MpoolAdd {
			continue
		}

		log.Info("mpool message")

		err := st.storeMessages(map[cid.Cid]*types.Message{
			change.Message.Message.Cid(): &change.Message.Message,
		})
		if err != nil {
			log.Error(err)
			continue
		}

		if err := st.storeMpoolInclusion(change.Message.Message.Cid()); err != nil {
			log.Error(err)
			continue
		}
	}
}
