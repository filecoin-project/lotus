package events

import (
	"context"
	"sync"

	"github.com/filecoin-project/lotus/api"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
)

type messageCache struct {
	api EventAPI

	blockMsgLk    sync.Mutex
	blockMsgCache *lru.ARCCache
}

func newMessageCache(api EventAPI) *messageCache {
	blsMsgCache, _ := lru.NewARC(500)

	return &messageCache{
		api:           api,
		blockMsgCache: blsMsgCache,
	}
}

func (c *messageCache) ChainGetBlockMessages(ctx context.Context, blkCid cid.Cid) (*api.BlockMessages, error) {
	c.blockMsgLk.Lock()
	defer c.blockMsgLk.Unlock()

	msgsI, ok := c.blockMsgCache.Get(blkCid)
	var err error
	if !ok {
		msgsI, err = c.api.ChainGetBlockMessages(ctx, blkCid)
		if err != nil {
			return nil, err
		}
		c.blockMsgCache.Add(blkCid, msgsI)
	}
	return msgsI.(*api.BlockMessages), nil
}
