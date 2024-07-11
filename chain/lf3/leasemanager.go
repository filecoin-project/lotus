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

// UpsertDefensive inserts or updates a lease for the given id to the expiration time either if:
// - old expiration is in the past
// - old expiration matches the one in leaseManager
// returns true if update has happened
func (lm *leaseManager) UpsertDefensive(id uint64, newExpiration time.Time, oldExpiration time.Time) bool {
	clk := lm.clk()
	if lm.leases == nil {
		lm.leases = make(map[uint64]time.Time)
	}
	// if the old lease is expired just insert a new one
	if clk.Until(oldExpiration) < 0 {
		lm.Upsert(id, newExpiration)
		return true
	}

	// old lease is not expired
	exp, ok := lm.leases[id]
	if !ok {
		// we don't know about it, don't start a new lease
		return false
	}
	if exp != oldExpiration {
		// the lease we know about does not match and because the old lease is not expired
		// we should not allow for new lease
		return false
	}
	// we know about the lease, update it
	lm.Upsert(id, newExpiration)
	return true
}

func (lm *leaseManager) clk() clock.Clock {
	if lm.clock != nil {
		return lm.clock
	}
	return clock.New()
}

// Active returns active leases and cleans up the inactive ones under the hood.
func (lm *leaseManager) Active() []uint64 {
	clk := lm.clk()
	var res []uint64
	for id, exp := range lm.leases {
		if clk.Until(exp) <= 0 {
			delete(lm.leases, id)
			continue
		}
		res = append(res, id)
	}
	return res
}
