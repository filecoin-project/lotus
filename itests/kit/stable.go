package kit

import (
	"context"
	"testing"

	"github.com/filecoin-project/lotus/chain/types"
)

// MakeStableExecute is a helper that will execute a function required by a test case
// repeatedly until the tipset observed before is the same as after execution of the
// function. This helps reduce flakies that come from reorgs between capturing the tipset
// and executing the function.
// Unfortunately it doesn't remove the problem entirely as we could have multiple reorgs
// off the tipset we observe and then back to it, but it should be extremely rare.
func MakeStableExecute(ctx context.Context, t *testing.T, getTipSet func(t *testing.T) *types.TipSet) func(fn func()) *types.TipSet {
	if getTipSet == nil {
		// Default to a no-op tipset getter which will always indicate stability
		getTipSet = func(t *testing.T) *types.TipSet { return nil }
	}

	return func(fn func()) *types.TipSet {
		// Create a stable execute function that takes the test context
		return func(t *testing.T) *types.TipSet {
			beforeTs := getTipSet(t)
			for {
				select {
				case <-ctx.Done():
					t.Fatalf("context cancelled during stable execution: %v", ctx.Err())
				default:
				}

				fn()

				afterTs := getTipSet(t)
				if beforeTs.Equals(afterTs) {
					// Chain hasn't changed during execution, safe to return
					return beforeTs
				}
				beforeTs = afterTs
			}
		}(t) // Pass the current test context
	}
}
