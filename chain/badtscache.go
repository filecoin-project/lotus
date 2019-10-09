package chain

import (
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
)

type BadBlockCache struct {
	lk        sync.Mutex
	badBlocks *lru.ARCCache
}

func NewBadBlockCache() *BadBlockCache {
	cache, err := lru.NewARC(8192)
	if err != nil {
		panic(err)
	}

	return &BadBlockCache{
		badBlocks: cache,
	}
}

func (bts *BadBlockCache) Add(c cid.Cid) {
	bts.lk.Lock()
	defer bts.lk.Unlock()
	bts.badBlocks.Add(c, nil)
}

func (bts *BadBlockCache) Has(c cid.Cid) bool {
	bts.lk.Lock()
	defer bts.lk.Unlock()
	return bts.badBlocks.Contains(c)
}
