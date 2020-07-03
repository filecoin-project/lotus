package main

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"

	aapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

func subMpool(ctx context.Context, api aapi.FullNode, st *storage) {
	sub, err := api.MpoolSub(ctx)
	if err != nil {
		return
	}

	for {
		var updates []aapi.MpoolUpdate

		select {
		case update := <-sub:
			updates = append(updates, update)
		case <-ctx.Done():
			return
		}

	loop:
		for {
			time.Sleep(10 * time.Millisecond)
			select {
			case update := <-sub:
				updates = append(updates, update)
			default:
				break loop
			}
		}

		msgs := map[cid.Cid]*types.Message{}
		for _, v := range updates {
			if v.Type != aapi.MpoolAdd {
				continue
			}

			msgs[v.Message.Message.Cid()] = &v.Message.Message
		}

		log.Debugf("Processing %d mpool updates", len(msgs))

		err := st.storeMessages(msgs)
		if err != nil {
			log.Error(err)
		}

		if err := st.storeMpoolInclusions(updates); err != nil {
			log.Error(err)
		}
	}
}
