package lf3

import (
	"testing"
	"time"

	"github.com/raulk/clock"
	"github.com/stretchr/testify/assert"
)

func TestLeaseManager_Upsert(t *testing.T) {
	lm := &leaseManager{
		clock: clock.NewMock(),
	}

	// Test inserting a new lease
	expiration := lm.clock.Now().Add(1 * time.Hour)
	lm.Upsert(1, expiration)
	assert.Equal(t, 1, len(lm.leases))
	assert.Equal(t, expiration, lm.leases[1])

	// Test updating an existing lease
	newExpiration := lm.clock.Now().Add(2 * time.Hour)
	lm.Upsert(1, newExpiration)
	assert.Equal(t, 1, len(lm.leases))
	assert.Equal(t, newExpiration, lm.leases[1])
}

func TestLeaseManager_Active(t *testing.T) {
	mockClock := clock.NewMock()
	lm := &leaseManager{
		clock: mockClock,
	}

	// Add some leases
	expiration1 := mockClock.Now().Add(1 * time.Hour)
	expiration2 := mockClock.Now().Add(2 * time.Hour)
	expiration3 := mockClock.Now().Add(-1 * time.Hour) // Already expired

	lm.Upsert(1, expiration1)
	lm.Upsert(2, expiration2)
	lm.Upsert(3, expiration3)

	// Check active leases before advancing the clock
	activeLeases := lm.Active()
	assert.ElementsMatch(t, []uint64{1, 2}, activeLeases)

	// Advance the clock and check active leases again
	mockClock.Add(1 * time.Hour)
	activeLeases = lm.Active()
	assert.ElementsMatch(t, []uint64{2}, activeLeases)

	mockClock.Add(1 * time.Hour)
	activeLeases = lm.Active()
	assert.Empty(t, activeLeases)
}

func TestLeaseManager_UpsertDefensive(t *testing.T) {
	mockClock := clock.NewMock()
	lm := &leaseManager{
		clock: mockClock,
	}

	// Test inserting a new lease when oldExpiration is in the past
	oldExpiration := mockClock.Now().Add(-1 * time.Hour)
	newExpiration := mockClock.Now().Add(1 * time.Hour)
	updated := lm.UpsertDefensive(1, newExpiration, oldExpiration)
	assert.True(t, updated)
	assert.Equal(t, newExpiration, lm.leases[1])

	// Test updating an existing lease when oldExpiration matches
	oldExpiration = newExpiration
	newExpiration = mockClock.Now().Add(2 * time.Hour)
	updated = lm.UpsertDefensive(1, newExpiration, oldExpiration)
	assert.True(t, updated)
	assert.Equal(t, newExpiration, lm.leases[1])

	// Test not updating a lease when oldExpiration does not match
	oldExpiration = mockClock.Now().Add(3 * time.Hour) // Different from the current lease expiration
	newExpiration = mockClock.Now().Add(4 * time.Hour)
	updated = lm.UpsertDefensive(1, newExpiration, oldExpiration)
	assert.False(t, updated)
	assert.NotEqual(t, newExpiration, lm.leases[1])

	// Test not updating a lease when it is not known
	unknownID := uint64(2)
	oldExpiration = mockClock.Now().Add(1 * time.Hour)
	newExpiration = mockClock.Now().Add(2 * time.Hour)
	updated = lm.UpsertDefensive(unknownID, newExpiration, oldExpiration)
	assert.False(t, updated)
	_, exists := lm.leases[unknownID]
	assert.False(t, exists)
}
