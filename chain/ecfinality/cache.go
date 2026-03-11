package ecfinality

import (
	"context"
	"errors"
	"math"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

// Provider is the interface for EC finality queries, implemented by
// ECFinalityCache for production and by mocks for testing.
type Provider interface {
	GetStatus(ctx context.Context) (*ECFinalityStatus, error)
	GetFinalizedTipSet(ctx context.Context) (*types.TipSet, error)
}

// ECFinalityStatus holds the resolved EC finality state for a given head.
type ECFinalityStatus struct {
	// ThresholdDepth is the shallowest epoch depth at which the reorg
	// probability drops below 2^-30. -1 if the threshold is not met.
	ThresholdDepth int
	// FinalizedTipSet is the tipset at that depth, or nil if not met.
	FinalizedTipSet *types.TipSet
	// Head is the chain head the computation was performed against.
	Head *types.TipSet
}

// ECFinalityCache caches the EC finality threshold depth, recomputing only
// when the chain head changes. The cost is dominated by the Skellam PMF
// calculation in FindThresholdDepth, not by tipset loading (which hits the
// ChainStore's ARC cache). The cache is safe for concurrent use.
type ECFinalityCache struct {
	cs         *store.ChainStore
	windowSize int // finality + 5 (the lookback the calculator needs)

	mu     sync.Mutex
	cached *ECFinalityStatus
}

// NewECFinalityCache creates a new cache backed by the given ChainStore.
// The finality parameter is the lookback depth for the L distribution
// (typically policy.ChainFinality = 900).
func NewECFinalityCache(cs *store.ChainStore, finality int) *ECFinalityCache {
	return &ECFinalityCache{
		cs:         cs,
		windowSize: finality + 5,
	}
}

// GetStatus returns the full EC finality state for the current head:
// threshold depth, finalized tipset, and the head used. The result is
// cached and recomputed only when the chain head changes.
//
// Errors are not cached; a transient failure allows retry on the next call.
// The computation uses a cancellation-resistant context so that a single
// caller's context cancellation does not poison the cache for subsequent
// callers.
func (c *ECFinalityCache) GetStatus(ctx context.Context) (*ECFinalityStatus, error) {
	head := c.cs.GetHeaviestTipSet()
	if head == nil {
		return nil, errors.New("no known heaviest tipset")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached result if head hasn't changed.
	if c.cached != nil && c.cached.Head.Key() == head.Key() {
		return c.cached, nil
	}

	// Detach from the caller's context so that cancellation of one request
	// does not prevent the result from being cached for others.
	computeCtx := context.WithoutCancel(ctx)

	chain, err := c.walkChain(computeCtx, head)
	if err != nil {
		return nil, err
	}

	guarantee := math.Pow(2, float64(DefaultSafetyExponent))
	threshold := FindThresholdDepth(chain, c.windowSize-5, DefaultBlocksPerEpoch, DefaultByzantineFraction, guarantee)

	status := &ECFinalityStatus{
		ThresholdDepth: threshold,
		Head:           head,
	}

	if threshold >= 0 {
		height := max(0, head.Height()-abi.ChainEpoch(threshold))
		ts, err := c.cs.GetTipsetByHeight(computeCtx, height, head, true)
		if err != nil {
			return nil, err
		}
		status.FinalizedTipSet = ts
	}

	c.cached = status
	return status, nil
}

// GetFinalizedTipSet returns the most recent tipset where the probability of
// reorg drops below 2^-30. Returns nil if the threshold is not met (chain may
// be degraded). This is a convenience wrapper; use GetStatus for the full
// finality breakdown.
func (c *ECFinalityCache) GetFinalizedTipSet(ctx context.Context) (*types.TipSet, error) {
	s, err := c.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	return s.FinalizedTipSet, nil
}

// walkChain walks back from head collecting block counts for the calculator.
// Each LoadTipSet call typically hits the ChainStore's ARC cache.
func (c *ECFinalityCache) walkChain(ctx context.Context, head *types.TipSet) ([]int, error) {
	needed := c.windowSize
	chain := make([]int, 0, needed)
	ts := head
	for len(chain) < needed {
		chain = append(chain, len(ts.Cids()))
		parent, err := c.cs.LoadTipSet(ctx, ts.Parents())
		if err != nil {
			return nil, err
		}
		ts = parent
	}
	// Reverse to chronological order (oldest first).
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}
