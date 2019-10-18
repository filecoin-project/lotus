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
		panic(err)
	}

	return &BadBlockCache{
		badBlocks: cache,
	}
}

func (bts *BadBlockCache) Add(c cid.Cid) {
	bts.badBlocks.Add(c, nil)
}

func (bts *BadBlockCache) Has(c cid.Cid) bool {
	return bts.badBlocks.Contains(c)
}
