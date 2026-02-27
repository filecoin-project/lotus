package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestBaseFee(t *testing.T) {
	tests := []struct {
		basefee             uint64
		limitUsed           int64
		noOfBlocks          int
		preSmoke, postSmoke uint64
	}{
		{100e6, 0, 1, 87.5e6, 87.5e6},
		{100e6, 0, 5, 87.5e6, 87.5e6},
		{100e6, buildconstants.BlockGasTarget, 1, 103.125e6, 100e6},
		{100e6, buildconstants.BlockGasTarget * 2, 2, 103.125e6, 100e6},
		{100e6, buildconstants.BlockGasLimit * 2, 2, 112.5e6, 112.5e6},
		{100e6, (buildconstants.BlockGasLimit * 15) / 10, 2, 110937500, 106.250e6},
	}

	for _, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%v", test), func(t *testing.T) {
			preSmoke := ComputeNextBaseFee(types.NewInt(test.basefee), test.limitUsed, test.noOfBlocks, buildconstants.UpgradeSmokeHeight-1)
			assert.Equal(t, fmt.Sprintf("%d", test.preSmoke), preSmoke.String())

			postSmoke := ComputeNextBaseFee(types.NewInt(test.basefee), test.limitUsed, test.noOfBlocks, buildconstants.UpgradeSmokeHeight+1)
			assert.Equal(t, fmt.Sprintf("%d", test.postSmoke), postSmoke.String())
		})
	}
}

// TestWeightedQuickSelect tests the tipset gas percentile cases.
// BlockGasLimit = 10_000_000_000, P = 20.
func TestWeightedQuickSelect(t *testing.T) {
	tests := []struct {
		premiums []int64
		limits   []int64
		expected int64
	}{
		{[]int64{123, 100}, []int64{5_999_999_999, 2_000_000_000}, 0},
		{[]int64{123, 0}, []int64{5_999_999_999, 2_000_000_001}, 0},
		{[]int64{123, 100}, []int64{5_999_999_999, 2_000_000_001}, 100},
		{[]int64{123, 100}, []int64{7_999_999_999, 2_000_000_001}, 100},
		{[]int64{123, 100}, []int64{8_000_000_000, 2_000_000_000}, 123},
		{[]int64{123, 100}, []int64{8_000_000_000, 9_000_000_000}, 123},
	}
	for _, tc := range tests {
		premiums := make([]abi.TokenAmount, len(tc.premiums))
		for i, p := range tc.premiums {
			premiums[i] = big.NewInt(p)
		}
		got := WeightedQuickSelect(premiums, tc.limits, buildconstants.BlockGasTargetIndex)
		assert.Equal(t, big.NewInt(tc.expected).String(), got.String(),
			"premiums=%v limits=%v", tc.premiums, tc.limits)
	}
}

// TestNextBaseFeeFromPremium tests the BaseFee_next formula:
// MaxAdj = ceil(BaseFee / 8)
// BaseFee_next = Max(MinBaseFee, BaseFee + Min(MaxAdj, Premium_P - MaxAdj))
func TestNextBaseFeeFromPremium(t *testing.T) {
	tests := []struct {
		baseFee  int64
		premiumP int64
		expected int64
	}{
		{100, 0, 100},
		{100, 13, 100},
		{100, 14, 101},
		{100, 26, 113},
		{801, 0, 700},
		{801, 20, 720},
		{801, 40, 740},
		{801, 60, 760},
		{801, 80, 780},
		{801, 100, 800},
		{801, 120, 820},
		{801, 140, 840},
		{801, 160, 860},
		{801, 180, 880},
		{801, 200, 900},
		{801, 201, 901},
		{808, 0, 707},
		{808, 1, 708},
		{808, 201, 908},
		{808, 202, 909},
		{808, 203, 909},
	}
	for _, tc := range tests {
		got := nextBaseFeeFromPremium(big.NewInt(tc.baseFee), big.NewInt(tc.premiumP))
		assert.Equal(t, big.NewInt(tc.expected).String(), got.String(),
			"baseFee=%d premiumP=%d", tc.baseFee, tc.premiumP)
	}
}
