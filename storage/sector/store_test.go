package sector

import (
	"gotest.tools/assert"
	"testing"
)

func TestComputePaddedSize(t *testing.T)  {
	assert.Equal(t, uint64(1040384), computePaddedSize(1000000))
	assert.Equal(t, uint64(1016), computePaddedSize(548))
	assert.Equal(t, uint64(4064), computePaddedSize(2048))
}