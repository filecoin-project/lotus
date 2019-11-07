package sectorbuilder_test

import (
	"io"
	"math/rand"
	"os"
	"testing"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

const sectorSize = 1024

func TestSealAndVerify(t *testing.T) {
	//t.Skip("this is slow")
	os.Setenv("BELLMAN_NO_GPU", "1")

	build.SectorSizes = []uint64{sectorSize}

	if err := build.GetParams(true); err != nil {
		t.Fatalf("%+v", err)
	}

	sb, cleanup, err := sectorbuilder.TempSectorbuilder(sectorSize)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	_ = cleanup
	//defer cleanup()

	dlen := sectorbuilder.UserBytesForSectorSize(sectorSize)

	sid, err := sb.AcquireSectorId()
	if err != nil {
		t.Fatalf("%+v", err)
	}

	r := io.LimitReader(rand.New(rand.NewSource(42)), int64(dlen))
	ppi, err := sb.AddPiece(dlen, sid, r, []uint64{})
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ticket := sectorbuilder.SealTicket{
		BlockHeight: 5,
		TicketBytes: [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2},
	}

	pco, err := sb.SealPreCommit(sid, ticket, []sectorbuilder.PublicPieceInfo{ppi})
	if err != nil {
		t.Fatalf("%+v", err)
	}

	seed := sectorbuilder.SealSeed{
		BlockHeight: 15,
		TicketBytes: [32]byte{0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9},
	}

	proof, err := sb.SealCommit(sid, ticket, seed, []sectorbuilder.PublicPieceInfo{ppi}, []string{"foo"}, pco)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ok, err := sectorbuilder.VerifySeal(sectorSize, pco.CommR[:], pco.CommD[:], sb.Miner, ticket.TicketBytes[:], seed.TicketBytes[:], sid, proof)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	if !ok {
		t.Fatal("proof failed to validate")
	}

	cSeed := [32]byte{0, 9, 2, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9}

	ssi := sectorbuilder.NewSortedSectorInfo([]sectorbuilder.SectorInfo{{
		SectorID: sid,
		CommR:    pco.CommR,
	}})

	postProof, err := sb.GeneratePoSt(ssi, cSeed, []uint64{})
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ok, err = sectorbuilder.VerifyPost(sb.SectorSize(), ssi, cSeed, postProof, []uint64{})
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if !ok {
		t.Fatal("bad post")
	}
}
