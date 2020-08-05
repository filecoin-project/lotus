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
		{100, 200, 10, 90},
		{100, 150, 30, 20},
		{1000, 1300, 240, 60},
		{500, 700, 140, 60},
		{200, 200, 0, 0},
		{20000, 21000, 1000, 0},
		{0, 2000, 0, 2000},
		{500, 651, 121, 30},
		{500, 5000, 0, 4500},
		{7499e6, 7500e6, 1000000, 0},
		{7500e6 / 2, 7500e6, 375000000, 3375000000},
		{1, 7500e6, 0, 7499999999},
	}

	for _, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%v", test), func(t *testing.T) {
			refund, toBurn := ComputeGasOutputs(test.used, test.limit)
			assert.Equal(t, test.refund, refund, "refund")
			assert.Equal(t, test.burn, toBurn, "burned")
		})
	}
}
