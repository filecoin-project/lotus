package chain

import (
	"sync"

	"github.com/ipfs/go-cid"
)

type BadBlockCache struct {
	lk        sync.Mutex
	badBlocks map[cid.Cid]struct{}
}

func NewBadBlockCache() *BadBlockCache {
	return &BadBlockCache{
		badBlocks: make(map[cid.Cid]struct{}),
	}
}

func (bts *BadBlockCache) Add(c cid.Cid) {
	bts.lk.Lock()
	defer bts.lk.Unlock()
	bts.badBlocks[c] = struct{}{}
}

func (bts *BadBlockCache) Has(c cid.Cid) bool {
	bts.lk.Lock()
	defer bts.lk.Unlock()
	_, ok := bts.badBlocks[c]
	return ok
}
