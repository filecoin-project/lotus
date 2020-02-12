package chain

import (
	"github.com/filecoin-project/lotus/build"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
)

type BadBlockCache struct {
	badBlocks *lru.ARCCache
}

func NewBadBlockCache() *BadBlockCache {
	cache, err := lru.NewARC(build.BadBlockCacheSize)
	if err != nil {
		panic(err) // ok
	}

	return &BadBlockCache{
		badBlocks: cache,
	}
}

func (bts *BadBlockCache) Add(c cid.Cid, reason string) {
	bts.badBlocks.Add(c, reason)
}

func (bts *BadBlockCache) Has(c cid.Cid) (string, bool) {
	rval, ok := bts.badBlocks.Get(c)
	if !ok {
		return "", false
	}

	return rval.(string), true
}
