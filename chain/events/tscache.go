package events

import (
	"context"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

type tsCacheAPI interface {
	ChainGetTipSetAfterHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)
	ChainHead(context.Context) (*types.TipSet, error)
}

// tipSetCache implements a simple ring-buffer cache to keep track of recent
// tipsets
type tipSetCache struct {
	mu sync.RWMutex

	byKey    map[types.TipSetKey]*types.TipSet
	byHeight []*types.TipSet
	start    int // chain head (end)
	len      int

	storage tsCacheAPI
}

func newTSCache(storage tsCacheAPI, cap abi.ChainEpoch) *tipSetCache {
	return &tipSetCache{
		byKey:    make(map[types.TipSetKey]*types.TipSet, cap),
		byHeight: make([]*types.TipSet, cap),
		start:    0,
		len:      0,

		storage: storage,
	}
}
func (tsc *tipSetCache) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	if ts, ok := tsc.byKey[tsk]; ok {
		return ts, nil
	}
	return tsc.storage.ChainGetTipSet(ctx, tsk)
}

func (tsc *tipSetCache) ChainGetTipSetByHeight(ctx context.Context, height abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	return tsc.get(ctx, height, tsk, true)
}

func (tsc *tipSetCache) ChainGetTipSetAfterHeight(ctx context.Context, height abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	return tsc.get(ctx, height, tsk, false)
}

func (tsc *tipSetCache) get(ctx context.Context, height abi.ChainEpoch, tsk types.TipSetKey, prev bool) (*types.TipSet, error) {
	fallback := tsc.storage.ChainGetTipSetAfterHeight
	if prev {
		fallback = tsc.storage.ChainGetTipSetByHeight
	}
	tsc.mu.RLock()

	// Nothing in the cache?
	if tsc.len == 0 {
		tsc.mu.RUnlock()
		log.Warnf("tipSetCache.get: cache is empty, requesting from storage (h=%d)", height)
		return fallback(ctx, height, tsk)
	}

	// Resolve the head.
	head := tsc.byHeight[tsc.start]
	if !tsk.IsEmpty() {
		// Not on this chain?
		var ok bool
		head, ok = tsc.byKey[tsk]
		if !ok {
			tsc.mu.RUnlock()
			return fallback(ctx, height, tsk)
		}
	}

	headH := head.Height()
	tailH := headH - abi.ChainEpoch(tsc.len)

	if headH == height {
		tsc.mu.RUnlock()
		return head, nil
	} else if headH < height {
		tsc.mu.RUnlock()
		// If the user doesn't pass a tsk, we assume "head" is the last tipset we processed.
		return nil, xerrors.Errorf("requested epoch is in the future")
	} else if height < tailH {
		log.Warnf("tipSetCache.get: requested tipset not in cache, requesting from storage (h=%d; tail=%d)", height, tailH)
		tsc.mu.RUnlock()
		return fallback(ctx, height, head.Key())
	}

	direction := 1
	if prev {
		direction = -1
	}
	var ts *types.TipSet
	for i := 0; i < tsc.len && ts == nil; i += direction {
		ts = tsc.byHeight[normalModulo(tsc.start-int(headH-height)+i, len(tsc.byHeight))]
	}
	tsc.mu.RUnlock()
	return ts, nil
}

func (tsc *tipSetCache) ChainHead(ctx context.Context) (*types.TipSet, error) {
	tsc.mu.RLock()
	best := tsc.byHeight[tsc.start]
	tsc.mu.RUnlock()
	if best == nil {
		return tsc.storage.ChainHead(ctx)
	}
	return best, nil
}

func (tsc *tipSetCache) add(to *types.TipSet) error {
	tsc.mu.Lock()
	defer tsc.mu.Unlock()

	if tsc.len > 0 {
		best := tsc.byHeight[tsc.start]
		if best.Height() >= to.Height() {
			return xerrors.Errorf("tipSetCache.add: expected new tipset height to be at least %d, was %d", tsc.byHeight[tsc.start].Height()+1, to.Height())
		}
		if best.Key() != to.Parents() {
			return xerrors.Errorf(
				"tipSetCache.add: expected new tipset %s (%d) to follow %s (%d), its parents are %s",
				to.Key(), to.Height(), best.Key(), best.Height(), best.Parents(),
			)
		}
	}

	nextH := to.Height()
	if tsc.len > 0 {
		nextH = tsc.byHeight[tsc.start].Height() + 1
	}

	// fill null blocks
	for nextH != to.Height() {
		tsc.start = normalModulo(tsc.start+1, len(tsc.byHeight))
		was := tsc.byHeight[tsc.start]
		if was != nil {
			tsc.byHeight[tsc.start] = nil
			delete(tsc.byKey, was.Key())
		}
		if tsc.len < len(tsc.byHeight) {
			tsc.len++
		}
		nextH++
	}

	tsc.start = normalModulo(tsc.start+1, len(tsc.byHeight))
	was := tsc.byHeight[tsc.start]
	if was != nil {
		delete(tsc.byKey, was.Key())
	}
	tsc.byHeight[tsc.start] = to
	if tsc.len < len(tsc.byHeight) {
		tsc.len++
	}
	tsc.byKey[to.Key()] = to
	return nil
}

func (tsc *tipSetCache) revert(from *types.TipSet) error {
	tsc.mu.Lock()
	defer tsc.mu.Unlock()

	return tsc.revertUnlocked(from)
}

func (tsc *tipSetCache) revertUnlocked(ts *types.TipSet) error {
	if tsc.len == 0 {
		return nil // this can happen, and it's fine
	}

	was := tsc.byHeight[tsc.start]

	if !was.Equals(ts) {
		return xerrors.New("tipSetCache.revert: revert tipset didn't match cache head")
	}
	delete(tsc.byKey, was.Key())

	tsc.byHeight[tsc.start] = nil
	tsc.start = normalModulo(tsc.start-1, len(tsc.byHeight))
	tsc.len--

	_ = tsc.revertUnlocked(nil) // revert null block gap
	return nil
}

func (tsc *tipSetCache) observer() TipSetObserver {
	return (*tipSetCacheObserver)(tsc)
}

type tipSetCacheObserver tipSetCache

var _ TipSetObserver = new(tipSetCacheObserver)

func (tsc *tipSetCacheObserver) Apply(_ context.Context, _, to *types.TipSet) error {
	return (*tipSetCache)(tsc).add(to)
}

func (tsc *tipSetCacheObserver) Revert(ctx context.Context, from, _ *types.TipSet) error {
	return (*tipSetCache)(tsc).revert(from)
}

func normalModulo(n, m int) int {
	return ((n % m) + m) % m
}
