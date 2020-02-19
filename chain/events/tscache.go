package events

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
)

type tsByHFunc func(context.Context, uint64, types.TipSetKey) (*types.TipSet, error)

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
		if tsc.cache[tsc.start].Height() >= ts.Height() {
			return xerrors.Errorf("tipSetCache.add: expected new tipset height to be at least %d, was %d", tsc.cache[tsc.start].Height()+1, ts.Height())
		}
	}

	nextH := ts.Height()
	if tsc.len > 0 {
		nextH = tsc.cache[tsc.start].Height() + 1
	}

	// fill null blocks
	for nextH != ts.Height() {
		tsc.start = normalModulo(tsc.start+1, len(tsc.cache))
		tsc.cache[tsc.start] = nil
		if tsc.len < len(tsc.cache) {
			tsc.len++
		}
		nextH++
	}

	tsc.start = normalModulo(tsc.start+1, len(tsc.cache))
	tsc.cache[tsc.start] = ts
	if tsc.len < len(tsc.cache) {
		tsc.len++
	}
	return nil
}

func (tsc *tipSetCache) revert(ts *types.TipSet) error {
	if tsc.len == 0 {
		return nil // this can happen, and it's fine
	}

	if !tsc.cache[tsc.start].Equals(ts) {
		return xerrors.New("tipSetCache.revert: revert tipset didn't match cache head")
	}

	tsc.cache[tsc.start] = nil
	tsc.start = normalModulo(tsc.start-1, len(tsc.cache))
	tsc.len--

	_ = tsc.revert(nil) // revert null block gap
	return nil
}

func (tsc *tipSetCache) getNonNull(height uint64) (*types.TipSet, error) {
	for {
		ts, err := tsc.get(height)
		if err != nil {
			return nil, err
		}
		if ts != nil {
			return ts, nil
		}
		height++
	}
}

func (tsc *tipSetCache) get(height uint64) (*types.TipSet, error) {
	if tsc.len == 0 {
		log.Warnf("tipSetCache.get: cache is empty, requesting from storage (h=%d)", height)
		return tsc.storage(context.TODO(), height, types.EmptyTSK)
	}

	headH := tsc.cache[tsc.start].Height()

	if height > headH {
		return nil, xerrors.Errorf("tipSetCache.get: requested tipset not in cache (req: %d, cache head: %d)", height, headH)
	}

	clen := len(tsc.cache)
	var tail *types.TipSet
	for i := 1; i <= tsc.len; i++ {
		tail = tsc.cache[normalModulo(tsc.start-tsc.len+i, clen)]
		if tail != nil {
			break
		}
	}

	if height < tail.Height() {
		log.Warnf("tipSetCache.get: requested tipset not in cache, requesting from storage (h=%d; tail=%d)", height, tail.Height())
		return tsc.storage(context.TODO(), height, tail.Key())
	}

	return tsc.cache[normalModulo(tsc.start-int(headH-height), clen)], nil
}

func (tsc *tipSetCache) best() *types.TipSet {
	return tsc.cache[tsc.start]
}

func normalModulo(n, m int) int {
	return ((n % m) + m) % m
}
