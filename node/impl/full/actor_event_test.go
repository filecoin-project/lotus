package full

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
)

func TestParseHeightRange(t *testing.T) {
	epochPtr := func(i int) *abi.ChainEpoch {
		e := abi.ChainEpoch(i)
		return &e
	}

	tcs := map[string]struct {
		heaviest abi.ChainEpoch
		from     *abi.ChainEpoch
		to       *abi.ChainEpoch
		maxRange abi.ChainEpoch
		minOut   abi.ChainEpoch
		maxOut   abi.ChainEpoch
		errStr   string
	}{
		"fails when both are specified and range is greater than max allowed range": {
			heaviest: 100,
			from:     epochPtr(256),
			to:       epochPtr(512),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "too large",
		},
		"fails when min is specified and range is greater than max allowed range": {
			heaviest: 500,
			from:     epochPtr(16),
			to:       nil,
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "'from' height is too far in the past",
		},
		"fails when max is specified and range is greater than max allowed range": {
			heaviest: 500,
			from:     nil,
			to:       epochPtr(65536),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "'to' height is too far in the future",
		},
		"fails when from is greater than to": {
			heaviest: 100,
			from:     epochPtr(512),
			to:       epochPtr(256),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "must be after",
		},
		"works when range is valid (nil from)": {
			heaviest: 500,
			from:     nil,
			to:       epochPtr(48),
			maxRange: 1000,
			minOut:   -1,
			maxOut:   48,
		},
		"works when range is valid (nil to)": {
			heaviest: 500,
			from:     epochPtr(0),
			to:       nil,
			maxRange: 1000,
			minOut:   0,
			maxOut:   -1,
		},
		"works when range is valid (nil from and to)": {
			heaviest: 500,
			from:     nil,
			to:       nil,
			maxRange: 1000,
			minOut:   -1,
			maxOut:   -1,
		},
		"works when range is valid and specified": {
			heaviest: 500,
			from:     epochPtr(16),
			to:       epochPtr(48),
			maxRange: 1000,
			minOut:   16,
			maxOut:   48,
		},
	}

	for name, tc := range tcs {
		tc2 := tc
		t.Run(name, func(t *testing.T) {
			min, max, err := parseHeightRange(tc2.heaviest, tc2.from, tc2.to, tc2.maxRange)
			require.Equal(t, tc2.minOut, min)
			require.Equal(t, tc2.maxOut, max)
			if tc2.errStr != "" {
				fmt.Println(err)
				require.Error(t, err)
				require.Contains(t, err.Error(), tc2.errStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
