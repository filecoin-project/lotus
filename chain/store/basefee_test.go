package store

import (
	"fmt"
	"testing"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/stretchr/testify/assert"
)

func TestBaseFee(t *testing.T) {
	tests := []struct {
		basefee    uint64
		limitUsed  int64
		noOfBlocks int
		output     uint64
	}{
		{100e6, 0, 1, 87.5e6},
		{100e6, 0, 5, 87.5e6},
		{100e6, build.BlockGasTarget, 1, 103.125e6},
		{100e6, build.BlockGasTarget * 2, 2, 103.125e6},
		{100e6, build.BlockGasLimit * 2, 2, 112.5e6},
		{100e6, build.BlockGasLimit * 1.5, 2, 110937500},
	}

	for _, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%v", test), func(t *testing.T) {
			output := ComputeNextBaseFee(types.NewInt(test.basefee), test.limitUsed, test.noOfBlocks, 0)
			assert.Equal(t, fmt.Sprintf("%d", test.output), output.String())
		})
	}
}
