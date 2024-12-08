package vm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStepGasCost(t *testing.T) {
	s := stepCost{
		{4, 103994170},
		{7, 112356810},
		{13, 122912610},
		{26, 137559930},
		{52, 162039100},
		{103, 210960780},
		{205, 318351180},
		{410, 528274980},
	}

	assert.EqualValues(t, 0, s.Lookup(0))
	assert.EqualValues(t, 0, s.Lookup(3))
	assert.EqualValues(t, 103994170, s.Lookup(4))
	assert.EqualValues(t, 103994170, s.Lookup(6))
	assert.EqualValues(t, 112356810, s.Lookup(7))
	assert.EqualValues(t, 210960780, s.Lookup(103))
	assert.EqualValues(t, 210960780, s.Lookup(204))
	assert.EqualValues(t, 318351180, s.Lookup(205))
	assert.EqualValues(t, 318351180, s.Lookup(409))
	assert.EqualValues(t, 528274980, s.Lookup(410))
	assert.EqualValues(t, 528274980, s.Lookup(10000000000))
}
