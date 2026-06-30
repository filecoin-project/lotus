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
		t.Run(fmt.Sprintf("%v", test), func(t *testing.T) {
			preSmoke := ComputeNextBaseFee(types.NewInt(test.basefee), test.limitUsed, test.noOfBlocks, buildconstants.UpgradeSmokeHeight-1)
			assert.Equal(t, fmt.Sprintf("%d", test.preSmoke), preSmoke.String())

			postSmoke := ComputeNextBaseFee(types.NewInt(test.basefee), test.limitUsed, test.noOfBlocks, buildconstants.UpgradeSmokeHeight+1)
			assert.Equal(t, fmt.Sprintf("%d", test.postSmoke), postSmoke.String())
		})
	}
}

// Test randomized algorithm by trying all permutations
type AllPermutations struct {
	t *testing.T
	// current level
	level int
	// known domain (n) at current level
	domain []int
	// current iteration positions (i < n) in the domain
	dfs []int
}

func (p *AllPermutations) Reset() {
	// same outputs should result in same inputs
	assert.Equal(p.t, p.level, len(p.domain), "nondeterminism detected (fewer queries)")

	for len(p.dfs) > 0 && p.dfs[len(p.dfs)-1]+1 == p.domain[len(p.domain)-1] {
		// pop finished domains
		p.domain = p.domain[:len(p.domain)-1]
		p.dfs = p.dfs[:len(p.dfs)-1]
	}
	if len(p.dfs) > 0 {
		p.dfs[len(p.dfs)-1]++
	}
	// next iteration in the permutation
	p.level = 0
}

func (p *AllPermutations) Done() bool {
	return len(p.domain) == 0
}

func (p *AllPermutations) Intn(n int) int {
	assert.True(p.t, n > 0, "Intn(0)")
	if p.level < len(p.domain) {
		// same outputs should result in same inputs
		assert.Equal(p.t, p.domain[p.level], n, "nondeterminism detected (different queries)")
	} else {
		// expand search domain
		p.domain = append(p.domain, n)
		p.dfs = append(p.dfs, 0)
	}
	p.level++
	return p.dfs[p.level-1]
}

// TestWeightedQuickSelect tests the tipset gas percentile cases.
// BlockGasLimit = 10_000_000_000, P = 20.
func TestWeightedQuickSelect(t *testing.T) {
	tests := []struct {
		premiums []int64
		limits   []int64
		expected int64
	}{
		{[]int64{}, []int64{}, 0},
		{[]int64{123, 100}, []int64{5_999_999_999, 2_000_000_000}, 0},
		{[]int64{123, 0}, []int64{5_999_999_999, 2_000_000_001}, 0},
		{[]int64{123, 100}, []int64{5_999_999_999, 2_000_000_001}, 100},
		{[]int64{123, 100}, []int64{7_999_999_999, 2_000_000_001}, 100},
		{[]int64{123, 100}, []int64{8_000_000_000, 2_000_000_000}, 123},
		{[]int64{123, 100}, []int64{8_000_000_000, 9_000_000_000}, 123},
		{[]int64{100, 200, 300, 400, 500, 600, 700}, []int64{4_000_000_000, 1_000_000_000, 2_000_000_000, 1_000_000_000, 2_000_000_000, 2_000_000_000, 3_000_000_000}, 400},
	}
	for _, tc := range tests {
		premiums := make([]abi.TokenAmount, len(tc.premiums))
		for i, p := range tc.premiums {
			premiums[i] = big.NewInt(p)
		}
		rand := &AllPermutations{}
		rand.t = t
		for {
			got := weightedQuickSelect(premiums, tc.limits, buildconstants.BlockGasTargetIndex, rand)
			assert.Equal(t, big.NewInt(tc.expected).String(), got.String(),
				"premiums=%v limits=%v", tc.premiums, tc.limits)
			rand.Reset()
			if rand.Done() {
				break
			}
		}
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
		got := NextBaseFeeFromPremium(big.NewInt(tc.baseFee), big.NewInt(tc.premiumP))
		assert.Equal(t, big.NewInt(tc.expected).String(), got.String(),
			"baseFee=%d premiumP=%d", tc.baseFee, tc.premiumP)
	}
}
