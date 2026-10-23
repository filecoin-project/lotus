package store

import (
	"context"
	"hash/maphash"
	"os"
	"strconv"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/shardedmutex"
)

// DefaultChainIndexCacheSize no longer sets the maximum size, just the initial size of the map.
var DefaultChainIndexCacheSize = 1 << 15

func init() {
	if s := os.Getenv("LOTUS_CHAIN_INDEX_CACHE"); s != "" {
		lcic, err := strconv.Atoi(s)
		if err != nil {
			log.Errorf("failed to parse 'LOTUS_CHAIN_INDEX_CACHE' env var: %s", err)
		}
		DefaultChainIndexCacheSize = lcic
	}

}

type ChainIndex struct {
	indexCache *xsync.MapOf[types.TipSetKey, *lbEntry]

	fillCacheLock shardedmutex.ShardedMutexFor[types.TipSetKey]

	loadTipSet loadTipSetFunc

	skipLength abi.ChainEpoch

	// readyTo is the lowest height Warm has made cheap to resolve, or -1.
	readyTo atomic.Int64
}
type loadTipSetFunc func(context.Context, types.TipSetKey) (*types.TipSet, error)

func maphashTSK(s maphash.Seed, tsk types.TipSetKey) uint64 {
	return maphash.Bytes(s, tsk.Bytes())
}

func NewChainIndex(lts loadTipSetFunc) *ChainIndex {
	ci := &ChainIndex{
		indexCache:    xsync.NewTypedMapOfPresized[types.TipSetKey, *lbEntry](maphashTSK, DefaultChainIndexCacheSize),
		fillCacheLock: shardedmutex.NewFor(maphashTSK, 32),
		loadTipSet:    lts,
		skipLength:    20,
	}
	ci.readyTo.Store(-1)
	return ci
}

type lbEntry struct {
	targetHeight abi.ChainEpoch
	target       types.TipSetKey
}

func (ci *ChainIndex) GetTipsetByHeight(ctx context.Context, from *types.TipSet, to abi.ChainEpoch) (*types.TipSet, error) {
	if from.Height()-to <= ci.skipLength {
		return ci.walkBack(ctx, from, to)
	}

	rounded, err := ci.roundDown(ctx, from)
	if err != nil {
		return nil, xerrors.Errorf("failed to round down: %w", err)
	}

	cur := rounded.Key()
	for {
		lbe, err := ci.entry(ctx, cur)
		if err != nil {
			return nil, err
		}

		if to == lbe.targetHeight {
			ts, err := ci.loadTipSet(ctx, lbe.target)
			if err != nil {
				return nil, xerrors.Errorf("failed to load tipset: %w", err)
			}

			return ts, nil
		}
		if to > lbe.targetHeight {
			ts, err := ci.loadTipSet(ctx, cur)
			if err != nil {
				return nil, xerrors.Errorf("failed to load tipset: %w", err)
			}
			return ci.walkBack(ctx, ts, to)
		}

		cur = lbe.target
	}
}

// Warm fills skip entries from `from` down to genesis so later lookups are
// cheap.
func (ci *ChainIndex) Warm(ctx context.Context, from *types.TipSet) error {
	rounded, err := ci.roundDown(ctx, from)
	if err != nil {
		return xerrors.Errorf("failed to round down: %w", err)
	}

	cur := rounded.Key()
	for {
		lbe, err := ci.entry(ctx, cur)
		if err != nil {
			return err
		}
		ci.readyTo.Store(int64(lbe.targetHeight))
		if lbe.targetHeight == 0 {
			return nil
		}
		cur = lbe.target
	}
}

// ReadyTo returns the lowest height Warm has made cheap to resolve, or -1
// before it has filled an entry.
func (ci *ChainIndex) ReadyTo() abi.ChainEpoch {
	return abi.ChainEpoch(ci.readyTo.Load())
}

// entry returns the skip entry for the rounded tipset tsk, filling it if
// missing.
func (ci *ChainIndex) entry(ctx context.Context, tsk types.TipSetKey) (*lbEntry, error) {
	if lbe, ok := ci.indexCache.Load(tsk); ok {
		return lbe, nil
	}

	lk := ci.fillCacheLock.GetLock(tsk)
	lk.Lock()
	defer lk.Unlock()
	// another caller may have filled it while we waited for the lock
	if lbe, ok := ci.indexCache.Load(tsk); ok {
		return lbe, nil
	}

	lbe, err := ci.fillCache(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to fill cache: %w", err)
	}
	ci.indexCache.Store(tsk, lbe)
	return lbe, nil
}

func (ci *ChainIndex) GetTipsetByHeightWithoutCache(ctx context.Context, from *types.TipSet, to abi.ChainEpoch) (*types.TipSet, error) {
	return ci.walkBack(ctx, from, to)
}

// Caller must hold indexCacheLk
func (ci *ChainIndex) fillCache(ctx context.Context, tsk types.TipSetKey) (*lbEntry, error) {
	ts, err := ci.loadTipSet(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load tipset: %w", err)
	}

	if ts.Height() == 0 {
		return &lbEntry{
			targetHeight: 0,
			target:       tsk,
		}, nil
	}

	// will either be equal to ts.Height, or at least > ts.Parent.Height()
	rheight := ci.roundHeight(ts.Height())

	parent, err := ci.loadTipSet(ctx, ts.Parents())
	if err != nil {
		return nil, err
	}

	rheight -= ci.skipLength
	if rheight < 0 {
		rheight = 0
	}

	var skipTarget *types.TipSet
	if parent.Height() < rheight {
		skipTarget = parent
	} else {
		skipTarget, err = ci.walkBack(ctx, parent, rheight)
		if err != nil {
			return nil, xerrors.Errorf("fillCache walkback: %w", err)
		}
	}

	lbe := &lbEntry{
		targetHeight: skipTarget.Height(),
		target:       skipTarget.Key(),
	}

	return lbe, nil
}

// floors to nearest skipLength multiple
func (ci *ChainIndex) roundHeight(h abi.ChainEpoch) abi.ChainEpoch {
	return (h / ci.skipLength) * ci.skipLength
}

func (ci *ChainIndex) roundDown(ctx context.Context, ts *types.TipSet) (*types.TipSet, error) {
	target := ci.roundHeight(ts.Height())

	rounded, err := ci.walkBack(ctx, ts, target)
	if err != nil {
		return nil, xerrors.Errorf("failed to walk back: %w", err)
	}

	return rounded, nil
}

func (ci *ChainIndex) walkBack(ctx context.Context, from *types.TipSet, to abi.ChainEpoch) (*types.TipSet, error) {
	if to > from.Height() {
		return nil, xerrors.Errorf("looking for tipset with height greater than start point")
	}

	if to == from.Height() {
		return from, nil
	}

	ts := from

	for {
		pts, err := ci.loadTipSet(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("failed to load tipset: %w", err)
		}

		if to > pts.Height() {
			// in case pts is lower than the epoch we're looking for (null blocks)
			// return a tipset above that height
			return ts, nil
		}
		if to == pts.Height() {
			return pts, nil
		}

		ts = pts
	}
}
