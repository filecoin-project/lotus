package padreader

import (
	"gotest.tools/assert"
	"testing"
)

func TestComputePaddedSize(t *testing.T) {
	assert.Equal(t, uint64(1040384), PaddedSize(1000000))
	assert.Equal(t, uint64(1016), PaddedSize(548))
	assert.Equal(t, uint64(4064), PaddedSize(2048))
}
