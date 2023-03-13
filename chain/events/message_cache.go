package events

import (
	"context"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/api"
)

type messageCache struct {
	api EventAPI

	blockMsgLk    sync.Mutex
	blockMsgCache *lru.ARCCache[cid.Cid, *api.BlockMessages]
}

func newMessageCache(a EventAPI) *messageCache {
	blsMsgCache, _ := lru.NewARC[cid.Cid, *api.BlockMessages](500)

	return &messageCache{
		api:           a,
		blockMsgCache: blsMsgCache,
	}
}

func (c *messageCache) ChainGetBlockMessages(ctx context.Context, blkCid cid.Cid) (*api.BlockMessages, error) {
	c.blockMsgLk.Lock()
	defer c.blockMsgLk.Unlock()

	msgs, ok := c.blockMsgCache.Get(blkCid)
	var err error
	if !ok {
		msgs, err = c.api.ChainGetBlockMessages(ctx, blkCid)
		if err != nil {
			return nil, err
		}
		c.blockMsgCache.Add(blkCid, msgs)
	}
	return msgs, nil
}
