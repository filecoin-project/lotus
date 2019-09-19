package events

import (
	"context"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/types"
)

type tsByHFunc func(context.Context, uint64, *types.TipSet) (*types.TipSet, error)

// tipSetCache implements a simple ring-buffer cache to keep track of recent
// tipsets
type tipSetCache struct {
	cache []*types.TipSet
	start int
	len   int

	storage tsByHFunc
}

func newTSCache(cap int, storage tsByHFunc) *tipSetCache {
	return &tipSetCache{
		cache: make([]*types.TipSet, cap),
		start: 0,
		len:   0,

		storage: storage,
	}
}

func (tsc *tipSetCache) add(ts *types.TipSet) error {
	if tsc.len > 0 {
		if tsc.cache[tsc.start].Height()+1 != ts.Height() {
			return xerrors.Errorf("tipSetCache.add: expected new tipset height to be %d, was %d", tsc.cache[tsc.start].Height()+1, ts.Height())
		}
	}

	tsc.start = (tsc.start + 1) % len(tsc.cache)
	tsc.cache[tsc.start] = ts
	if tsc.len < len(tsc.cache) {
		tsc.len++
	}
	return nil
}

func (tsc *tipSetCache) revert(ts *types.TipSet) error {
	if tsc.len == 0 {
		return xerrors.New("tipSetCache.revert: nothing to revert; cache is empty")
	}

	if !tsc.cache[tsc.start].Equals(ts) {
		return xerrors.New("tipSetCache.revert: revert tipset didn't match cache head")
	}

	tsc.cache[tsc.start] = nil
	tsc.start = (tsc.start - 1) % len(tsc.cache)
	tsc.len--
	return nil
}

func (tsc *tipSetCache) get(height uint64) (*types.TipSet, error) {
	if tsc.len == 0 {
		return nil, xerrors.New("tipSetCache.get: cache is empty")
	}

	headH := tsc.cache[tsc.start].Height()

	if height > headH {
		return nil, xerrors.Errorf("tipSetCache.get: requested tipset not in cache (req: %d, cache head: %d)", height, headH)
	}

	clen := len(tsc.cache)
	tailH := tsc.cache[((tsc.start-tsc.len+1)% clen + clen) % clen].Height()

	if height < tailH {
		return tsc.storage(context.TODO(), height, tsc.cache[tailH])
	}

	return tsc.cache[(int(height-tailH+1) % clen + clen) % clen], nil
}

func (tsc *tipSetCache) best() *types.TipSet {
	return tsc.cache[tsc.start]
}
