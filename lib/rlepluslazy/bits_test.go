package rlepluslazy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunsFromBits(t *testing.T) {
	expected := []Run{Run{Val: false, Len: 0x1},
		{Val: true, Len: 0x3},
		{Val: false, Len: 0x2},
		{Val: true, Len: 0x3},
	}
	rit, err := RunsFromBits(BitsFromSlice([]uint64{1, 2, 3, 6, 7, 8}))
	assert.NoError(t, err)
	i := 10
	output := make([]Run, 0, 4)
	for rit.HasNext() && i > 0 {
		run, err := rit.NextRun()
		assert.NoError(t, err)
		i--
		output = append(output, run)
	}
	assert.NotEqual(t, 0, i, "too many iterations")
	assert.Equal(t, expected, output)
}
