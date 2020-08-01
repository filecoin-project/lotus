package vm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGasBurn(t *testing.T) {
	tests := []struct {
		used   int64
		limit  int64
		refund int64
		burn   int64
	}{
		{100, 200, 30, 70},
		{1000, 1300, 300, 0},
		{500, 700, 150, 50},
		{200, 200, 0, 0},
		{20000, 21000, 1000, 0},
		{0, 2000, 0, 2000},
		{500, 651, 150, 1},
	}

	for _, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%v", test), func(t *testing.T) {
			refund, toBurn := ComputeGasOutputs(test.used, test.limit)
			assert.Equal(t, test.burn, toBurn)
			assert.Equal(t, test.refund, refund)
		})
	}
}
