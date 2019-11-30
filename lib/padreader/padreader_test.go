package padreader

import (
	"testing"

	"gotest.tools/assert"
)

func TestComputePaddedSize(t *testing.T) {
	assert.Equal(t, uint64(1040384), PaddedSize(1000000))

	assert.Equal(t, uint64(1016), PaddedSize(548))
	assert.Equal(t, uint64(1016), PaddedSize(1015))
	assert.Equal(t, uint64(1016), PaddedSize(1016))
	assert.Equal(t, uint64(2032), PaddedSize(1017))

	assert.Equal(t, uint64(2032), PaddedSize(1024))
	assert.Equal(t, uint64(4064), PaddedSize(2048))
}
