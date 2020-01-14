package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sectorbuilder "github.com/xjrwfilecoin/go-sectorbuilder"
)

func testFill(t *testing.T, n uint64, exp []uint64) {
	f, err := fillersFromRem(n)
	assert.NoError(t, err)
	assert.Equal(t, exp, f)

	var sum uint64
	for _, u := range f {
		sum += u
	}
	assert.Equal(t, n, sum)
}

func TestFillersFromRem(t *testing.T) {
	for i := 8; i < 32; i++ {
		// single
		ub := sectorbuilder.UserBytesForSectorSize(uint64(1) << i)
		testFill(t, ub, []uint64{ub})

		// 2
		ub = sectorbuilder.UserBytesForSectorSize(uint64(5) << i)
		ub1 := sectorbuilder.UserBytesForSectorSize(uint64(1) << i)
		ub3 := sectorbuilder.UserBytesForSectorSize(uint64(4) << i)
		testFill(t, ub, []uint64{ub1, ub3})

		// 4
		ub = sectorbuilder.UserBytesForSectorSize(uint64(15) << i)
		ub2 := sectorbuilder.UserBytesForSectorSize(uint64(2) << i)
		ub4 := sectorbuilder.UserBytesForSectorSize(uint64(8) << i)
		testFill(t, ub, []uint64{ub1, ub2, ub3, ub4})

		// different 2
		ub = sectorbuilder.UserBytesForSectorSize(uint64(9) << i)
		testFill(t, ub, []uint64{ub1, ub4})
	}

}
