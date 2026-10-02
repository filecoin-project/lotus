package eth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGasSearchWithCap(t *testing.T) {
	for _, tc := range []struct {
		name      string
		gasUsed   int64
		gasCap    int64
		minimum   int64
		want      int64
		wantBelow bool
	}{
		{name: "seed succeeds", gasUsed: 100, gasCap: 1000, minimum: 100, want: 125},
		{name: "safety margin reaches cap", gasUsed: 900, gasCap: 1000, minimum: 900, want: 1000},
		{name: "lower probes fail", gasUsed: 100, gasCap: 1000, minimum: 300, wantBelow: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The caller has already proved that execution succeeds at the cap.
			successful := map[int64]bool{tc.gasCap: true}
			var failedProbes int
			result, err := gasSearchWithCap(tc.gasUsed, tc.gasCap, 1.25, func(limit int64) (bool, error) {
				require.Positive(t, limit)
				require.LessOrEqual(t, limit, tc.gasCap)
				ok := limit >= tc.minimum
				if ok {
					successful[limit] = true
				} else {
					// Failed execution need not carry an out-of-gas error.
					failedProbes++
				}
				return ok, nil
			})
			require.NoError(t, err)
			require.True(t, successful[result], "returned limit must have succeeded")
			if tc.wantBelow {
				require.Positive(t, failedProbes)
				require.GreaterOrEqual(t, result, tc.minimum)
				require.Less(t, result, tc.gasCap)
			} else {
				require.Equal(t, tc.want, result)
			}
		})
	}
}

func TestGasSearchWithCapPaddingCanFail(t *testing.T) {
	const gasCap int64 = 1000
	successful := map[int64]bool{gasCap: true}
	var paddingFailed bool
	result, err := gasSearchWithCap(100, gasCap, 1.25, func(limit int64) (bool, error) {
		require.Positive(t, limit)
		require.LessOrEqual(t, limit, gasCap)
		// A gas-sensitive contract can fail with more gas than a successful call.
		if limit >= 375 && limit <= 500 {
			paddingFailed = true
			return false, nil
		}
		ok := limit >= 300
		if ok {
			successful[limit] = true
		}
		return ok, nil
	})
	require.NoError(t, err)
	require.True(t, paddingFailed, "test must exercise the failed padding probe")
	require.True(t, successful[result], "retain the previously successful limit")
	require.GreaterOrEqual(t, result, int64(300))
	require.Less(t, result, int64(375))
}

func TestGasSearchWithCapPropagatesError(t *testing.T) {
	wantErr := errors.New("simulation failed")
	_, err := gasSearchWithCap(100, 1000, 1.25, func(int64) (bool, error) {
		return false, wantErr
	})
	require.ErrorIs(t, err, wantErr)
}
