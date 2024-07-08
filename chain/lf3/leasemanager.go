package lf3

import (
	"time"

	"github.com/raulk/clock"
)

type leaseManager struct {
	// clock for testing
	clock  clock.Clock
	leases map[uint64]time.Time
}

// Upsert inserts or updates a lease for given id to the expiration time.
func (lm *leaseManager) Upsert(id uint64, expiration time.Time) {
	if lm.leases == nil {
		lm.leases = make(map[uint64]time.Time)
	}
	lm.leases[id] = expiration
}

func (lm *leaseManager) clk() clock.Clock {
	if lm.clock != nil {
		return lm.clock
	}
	return clock.New()
}

// Active returns active leases and cleans up the inactive ones under the hood.
func (lm *leaseManager) Active() []uint64 {
	var res []uint64
	for id, exp := range lm.leases {
		if lm.clk().Until(exp) <= 0 {
			delete(lm.leases, id)
			continue
		}
		res = append(res, id)
	}
	return res
}
