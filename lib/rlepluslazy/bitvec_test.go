package rlepluslazy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadBitVec(t *testing.T) {
	buf := []byte{0x0, 0xff}
	bv := readBitvec(buf)

	o := bv.Get(1)
	assert.EqualValues(t, 0, o)

	o = bv.Get(8)
	assert.EqualValues(t, 0x80, o)

	o = bv.Get(7)
	assert.EqualValues(t, 0x7f, o)
}
