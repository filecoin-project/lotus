package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

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
