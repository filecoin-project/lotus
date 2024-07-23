package chain

import (
	"fmt"

	"github.com/hashicorp/golang-lru/arc/v2"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

type BadBlockCache struct {
	badBlocks *arc.ARCCache[cid.Cid, BadBlockReason]
}

type BadBlockReason struct {
	Reason         string
	TipSet         []cid.Cid
	OriginalReason *BadBlockReason
}

func NewBadBlockReason(cid []cid.Cid, format string, i ...interface{}) BadBlockReason {
	return BadBlockReason{
		TipSet: cid,
		Reason: fmt.Sprintf(format, i...),
	}
}

func (bbr BadBlockReason) Linked(reason string, i ...interface{}) BadBlockReason {
	or := &bbr
	if bbr.OriginalReason != nil {
		or = bbr.OriginalReason
	}
	return BadBlockReason{Reason: fmt.Sprintf(reason, i...), OriginalReason: or}
}

func (bbr BadBlockReason) String() string {
	res := bbr.Reason
	if bbr.OriginalReason != nil {
		res += " caused by: " + fmt.Sprintf("%s %s", bbr.OriginalReason.TipSet, bbr.OriginalReason.String())
	}
	return res
}

func NewBadBlockCache() *BadBlockCache {
	cache, err := arc.NewARC[cid.Cid, BadBlockReason](buildconstants.BadBlockCacheSize)
	if err != nil {
		panic(err) // ok
	}

	return &BadBlockCache{
		badBlocks: cache,
	}
}

func (bts *BadBlockCache) Add(c cid.Cid, bbr BadBlockReason) {
	bts.badBlocks.Add(c, bbr)
}

func (bts *BadBlockCache) Remove(c cid.Cid) {
	bts.badBlocks.Remove(c)
}

func (bts *BadBlockCache) Purge() {
	bts.badBlocks.Purge()
}

func (bts *BadBlockCache) Has(c cid.Cid) (BadBlockReason, bool) {
	return bts.badBlocks.Get(c)
}
