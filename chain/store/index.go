package store

import (
	"context"
	"os"
	"strconv"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	stripedmutex "github.com/nmvalera/striped-mutex"
)

var DefaultChainIndexCacheSize = 32 << 15

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
	indexCache   *lru.ARCCache[types.TipSetKey, *lbEntry]
	indexCacheLk *stripedmutex.StripedMutex

	loadTipSet loadTipSetFunc

	skipLength abi.ChainEpoch
}
type loadTipSetFunc func(context.Context, types.TipSetKey) (*types.TipSet, error)

func NewChainIndex(lts loadTipSetFunc) *ChainIndex {
	sc, _ := lru.NewARC[types.TipSetKey, *lbEntry](DefaultChainIndexCacheSize)
	return &ChainIndex{
		indexCache:   sc,
		indexCacheLk: stripedmutex.New(32),
		loadTipSet:   lts,
		skipLength:   20,
	}
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
		// To prevent redundant computation in concurrent scenarios for the same tipset, we employ a striped lock.
		// This lock uses the tipset itself as a key, thereby ensuring mutual exclusion on a per-tipset basis.
		// As a result, duplicate work is avoided because the concurrency level for any specific tipset is maintained at 1.
		ci.indexCacheLk.Lock(cur.String())
		lbe, ok := ci.indexCache.Get(cur)
		if !ok {
			fc, err := ci.fillCache(ctx, cur)
			if err != nil {
				ci.indexCacheLk.Unlock(cur.String())
				return nil, xerrors.Errorf("failed to fill cache: %w", err)
			}
			lbe = fc
		}
		ci.indexCacheLk.Unlock(cur.String())

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

func (ci *ChainIndex) GetTipsetByHeightWithoutCache(ctx context.Context, from *types.TipSet, to abi.ChainEpoch) (*types.TipSet, error) {
	return ci.walkBack(ctx, from, to)
}

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
	ci.indexCache.Add(tsk, lbe)

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
