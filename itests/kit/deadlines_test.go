package kit

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
)

// TestDeadlineNotAfter tests the clamp behind CurrentProvingDeadline.
func TestDeadlineNotAfter(t *testing.T) {
	const (
		period      = abi.ChainEpoch(2880)
		window      = abi.ChainEpoch(60)
		periodStart = abi.ChainEpoch(658)
	)
	mk := func(index uint64, cur abi.ChainEpoch) *dline.Info {
		return dline.NewInfo(periodStart, index, cur, 48, period, window, 20, 70)
	}

	t.Run("deadline containing height is unchanged", func(t *testing.T) {
		di := mk(37, periodStart+37*window+5)
		require.Same(t, di, DeadlineNotAfter(di, di.CurrentEpoch))
	})

	t.Run("deadline before height is unchanged", func(t *testing.T) {
		di := mk(37, periodStart+38*window)
		require.Same(t, di, DeadlineNotAfter(di, di.CurrentEpoch))
	})

	// On a deadline close when the preceding epochs were null rounds,
	// StateMinerProvingDeadline returns that the recorded deadline has elapsed, so
	// NextNotElapsed moves it to the next proving period with the same index.
	for _, tc := range []struct {
		name  string
		index uint64
		nulls abi.ChainEpoch
	}{
		{"one null round", 37, 1},
		{"two null rounds", 37, 2},
		{"one null round at the last deadline of the period", 47, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorded := mk(tc.index, periodStart+abi.ChainEpoch(tc.index)*window)
			height := recorded.Close + tc.nulls - 1
			jumped := dline.NewInfo(periodStart, tc.index, height, 48, period, window, 20, 70).NextNotElapsed()
			require.Equal(t, periodStart+period, jumped.PeriodStart)
			require.Greater(t, jumped.Open, height)

			got := DeadlineNotAfter(jumped, height)
			require.Equal(t, periodStart, got.PeriodStart)
			require.Equal(t, tc.index, got.Index)
			require.Equal(t, recorded.Open, got.Open)
			require.Equal(t, recorded.Close, got.Close)
			require.Equal(t, height, got.CurrentEpoch)
		})
	}

	t.Run("rewinds multiple periods", func(t *testing.T) {
		height := periodStart + 37*window
		di := dline.NewInfo(periodStart+3*period, 37, height, 48, period, window, 20, 70)
		got := DeadlineNotAfter(di, height)
		require.Equal(t, periodStart, got.PeriodStart)
		require.Equal(t, height, got.Open)
	})
}

func TestDeadlineForHeight(t *testing.T) {
	const (
		period = abi.ChainEpoch(2880)
		window = abi.ChainEpoch(60)
	)
	mk := func(periodStart abi.ChainEpoch, index uint64, cur abi.ChainEpoch) *dline.Info {
		return dline.NewInfo(periodStart, index, cur, 48, period, window, 20, 70)
	}

	for _, tc := range []struct {
		name        string
		periodStart abi.ChainEpoch
		recorded    uint64
		height      abi.ChainEpoch
		want        uint64
		wantOpen    abi.ChainEpoch
	}{
		// A null round at the close of deadline 47 leaves NextNotElapsed reporting index 47 in the next
		// period, and height 2880 is the first epoch of deadline 0 of that period.
		{"stale index 47 at a period boundary", 0, 47, 2880, 0, 2880},
		// Same shape one deadline earlier: selection must target 38, not the recorded 37.
		{"stale index 37 at the close of 37", 0, 37, 2280, 38, 2280},
		// An interior epoch of a deadline names that deadline, not the next one.
		{"interior epoch of a deadline", 0, 37, 2300, 38, 2280},
		// A miner out of cron records a deadline that has not opened yet; no rewind is owed.
		{"recorded deadline ahead of height", 0, 30, 600, 10, 600},
		// A miner whose first proving period has not started yet, the schedule's phase still decides
		// which deadline an epoch belongs to.
		{"period start ahead of height", 1200, 0, 600, 38, 600},
		// Before the schedule's own origin where a raw quotient is negative.
		{"height before the period start", 0, 0, -1, 47, -60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorded := mk(tc.periodStart, tc.recorded, tc.height)
			if recorded.Open <= tc.height {
				recorded = recorded.NextNotElapsed()
			}

			got := DeadlineForHeight(recorded, tc.height)

			require.Equal(t, tc.want, got.Index, "deadline index")
			require.Equal(t, tc.wantOpen, got.Open, "deadline open")
			require.Equal(t, tc.wantOpen+window, got.Close, "close must belong to the same deadline as open")
			require.Equal(t, tc.wantOpen-20, got.Challenge, "challenge must belong to the same deadline as open")
			require.Less(t, got.Index, uint64(48), "index must be a real deadline")
			require.LessOrEqual(t, got.Open, tc.height, "the deadline must have opened")
			require.Greater(t, got.Close, tc.height, "the deadline must not have closed")
			require.Equal(t, tc.height, got.CurrentEpoch)

			// The exported index helper takes raw API results, so it must reach the same answer
			// without the caller normalising first.
			require.Equal(t, tc.want, CurrentDeadlineIndex(recorded), "index from the raw deadline")
			require.Equal(t, tc.want, CurrentDeadlineIndex(got), "index from the derived deadline")
		})
	}
}
