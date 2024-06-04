package filler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-state-types/abi"
)

func testFill(t *testing.T, n abi.UnpaddedPieceSize, exp []abi.UnpaddedPieceSize) {
	f, err := FillersFromRem(n)
	assert.NoError(t, err)
	assert.Equal(t, exp, f)

	var sum abi.UnpaddedPieceSize
	for _, u := range f {
		sum += u
	}
	assert.Equal(t, n, sum)
}

func TestFillersFromRem(t *testing.T) {
	for i := 8; i < 32; i++ {
		// single
		ub := abi.PaddedPieceSize(uint64(1) << i).Unpadded()
		testFill(t, ub, []abi.UnpaddedPieceSize{ub})

		// 2
		ub = abi.PaddedPieceSize(uint64(5) << i).Unpadded()
		ub1 := abi.PaddedPieceSize(uint64(1) << i).Unpadded()
		ub3 := abi.PaddedPieceSize(uint64(4) << i).Unpadded()
		testFill(t, ub, []abi.UnpaddedPieceSize{ub1, ub3})

		// 4
		ub = abi.PaddedPieceSize(uint64(15) << i).Unpadded()
		ub2 := abi.PaddedPieceSize(uint64(2) << i).Unpadded()
		ub4 := abi.PaddedPieceSize(uint64(8) << i).Unpadded()
		testFill(t, ub, []abi.UnpaddedPieceSize{ub1, ub2, ub3, ub4})

		// different 2
		ub = abi.PaddedPieceSize(uint64(9) << i).Unpadded()
		testFill(t, ub, []abi.UnpaddedPieceSize{ub1, ub4})
	}
}
