package store

import "time"

// SetHeightLookupBudget overrides the GetTipsetByHeightBounded budget until
// the returned restore func is called.
func SetHeightLookupBudget(d time.Duration) (restore func()) {
	old := heightLookupBudget
	heightLookupBudget = d
	return func() { heightLookupBudget = old }
}
