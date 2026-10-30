package store

import (
	"context"
	"time"

	"github.com/filecoin-project/lotus/chain/types"
)

// SetHeightLookupBudget overrides the GetTipsetByHeightBounded budget until
// the returned restore func is called.
func SetHeightLookupBudget(d time.Duration) (restore func()) {
	old := heightLookupBudget
	heightLookupBudget = d
	return func() { heightLookupBudget = old }
}

// WarmHeightIndexSync warms the height index from the current head in the
// foreground.
func (cs *ChainStore) WarmHeightIndexSync(ctx context.Context) error {
	return cs.cindex.Warm(ctx, cs.GetHeaviestTipSet())
}

// FailHeightIndexLoadsOnDone makes height index tipset loads fail once their
// context is done, as loads from a slow blockstore would.
func (cs *ChainStore) FailHeightIndexLoadsOnDone() {
	load := cs.cindex.loadTipSet
	cs.cindex.loadTipSet = func(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return load(ctx, tsk)
	}
}
