package sectorbuilder_test

import (
	"context"
	"io"
	"math/rand"
	"testing"

	"github.com/ipfs/go-datastore"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/storage/sector"
)

const sectorSize = 1024

func TestSealAndVerify(t *testing.T) {
	t.Skip("this is slow")
	build.SectorSizes = []uint64{sectorSize}

	if err := build.GetParams(true); err != nil {
		t.Fatal(err)
	}

	sb, cleanup, err := sectorbuilder.TempSectorbuilder(sectorSize)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// TODO: Consider fixing
	store := sector.NewStore(sb, datastore.NewMapDatastore(), func(ctx context.Context) (*sectorbuilder.SealTicket, error) {
		return &sectorbuilder.SealTicket{
			BlockHeight: 5,
			TicketBytes: [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2},
		}, nil
	})

	store.Service()

	dlen := sectorbuilder.UserBytesForSectorSize(sectorSize)

	r := io.LimitReader(rand.New(rand.NewSource(42)), int64(dlen))
	sid, err := store.AddPiece("foo", dlen, r)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SealSector(context.TODO(), sid); err != nil {
		t.Fatal(err)
	}

	ssinfo := <-store.Incoming()

	ok, err := sectorbuilder.VerifySeal(sectorSize, ssinfo.CommR[:], ssinfo.CommD[:], sb.Miner, ssinfo.Ticket.TicketBytes[:], ssinfo.SectorID, ssinfo.Proof)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("proof failed to validate")
	}
}
