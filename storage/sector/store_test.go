package sector

import (
	"context"
	"fmt"
	"github.com/filecoin-project/lotus/lib/padreader"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/ipfs/go-datastore"
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

func TestSectorStore(t *testing.T) {
	if err := build.GetParams(true); err != nil {
		t.Fatal(err)
	}

	sb, cleanup, err := sectorbuilder.TempSectorbuilder(1024)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	tktFn := func(context.Context) (*sectorbuilder.SealTicket, error) {
		return &sectorbuilder.SealTicket{
			BlockHeight: 17,
			TicketBytes: [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2},
		}, nil
	}

	ds := datastore.NewMapDatastore()

	store := NewStore(sb, ds, tktFn)

	pr := io.LimitReader(rand.New(rand.NewSource(17)), 300)
	pr, n := padreader.New(pr, 300)

	sid, err := store.AddPiece("a", n, pr, 1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(sid)
}
