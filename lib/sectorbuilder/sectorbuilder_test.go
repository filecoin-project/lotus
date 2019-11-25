package sectorbuilder_test

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

const sectorSize = 1024

type seal struct {
	sid uint64

	pco sectorbuilder.RawSealPreCommitOutput
	ppi sectorbuilder.PublicPieceInfo

	ticket sectorbuilder.SealTicket
}

func (s *seal) precommit(t *testing.T, sb *sectorbuilder.SectorBuilder, sid uint64, done func()) {
	dlen := sectorbuilder.UserBytesForSectorSize(sectorSize)

	var err error
	r := io.LimitReader(rand.New(rand.NewSource(42+int64(sid))), int64(dlen))
	s.ppi, err = sb.AddPiece(dlen, sid, r, []uint64{})
	if err != nil {
		t.Fatalf("%+v", err)
	}

	s.ticket = sectorbuilder.SealTicket{
		BlockHeight: 5,
		TicketBytes: [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2},
	}

	s.pco, err = sb.SealPreCommit(sid, s.ticket, []sectorbuilder.PublicPieceInfo{s.ppi})
	if err != nil {
		t.Fatalf("%+v", err)
	}

	done()
}

func (s *seal) commit(t *testing.T, sb *sectorbuilder.SectorBuilder, done func()) {
	seed := sectorbuilder.SealSeed{
		BlockHeight: 15,
		TicketBytes: [32]byte{0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9},
	}

	proof, err := sb.SealCommit(s.sid, s.ticket, seed, []sectorbuilder.PublicPieceInfo{s.ppi}, []string{"foo"}, s.pco)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ok, err := sectorbuilder.VerifySeal(sectorSize, s.pco.CommR[:], s.pco.CommD[:], sb.Miner, s.ticket.TicketBytes[:], seed.TicketBytes[:], s.sid, proof)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	if !ok {
		t.Fatal("proof failed to validate")
	}

	done()
}

func (s *seal) post(t *testing.T, sb *sectorbuilder.SectorBuilder) {
	cSeed := [32]byte{0, 9, 2, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9}

	ssi := sectorbuilder.NewSortedPublicSectorInfo([]sectorbuilder.PublicSectorInfo{{
		SectorID: s.sid,
		CommR:    s.pco.CommR,
	}})

	candndates, err := sb.GenerateEPostCandidates(ssi, cSeed, []uint64{})
	if err != nil {
		t.Fatalf("%+v", err)
	}

	postProof, err := sb.ComputeElectionPoSt(ssi, cSeed[:], candndates)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ok, err := sectorbuilder.VerifyPost(context.TODO(), sb.SectorSize(), ssi, cSeed[:], postProof, candndates, sb.Miner)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if !ok {
		t.Fatal("bad post")
	}
}

func TestSealAndVerify(t *testing.T) {
	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	os.Setenv("BELLMAN_NO_GPU", "1")
	os.Setenv("RUST_LOG", "info")

	build.SectorSizes = []uint64{sectorSize}

	if err := build.GetParams(true, true); err != nil {
		t.Fatalf("%+v", err)
	}

	ds := datastore.NewMapDatastore()

	dir, err := ioutil.TempDir("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	sb, err := sectorbuilder.TempSectorbuilderDir(dir, sectorSize, ds)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	cleanup := func() {
		sb.Destroy()
		if t.Failed() {
			fmt.Printf("not removing %s\n", dir)
			return
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	}
	defer cleanup()

	si, err := sb.AcquireSectorId()
	if err != nil {
		t.Fatalf("%+v", err)
	}

	s := seal{sid: si}

	s.precommit(t, sb, 1, func() {})

	s.commit(t, sb, func() {})

	s.post(t, sb)

	// Restart sectorbuilder, re-run post
	sb.Destroy()
	sb, err = sectorbuilder.TempSectorbuilderDir(dir, sectorSize, ds)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	s.post(t, sb)
}

func TestSealAndVerify2(t *testing.T) {
	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	os.Setenv("BELLMAN_NO_GPU", "1")
	os.Setenv("RUST_LOG", "info")

	build.SectorSizes = []uint64{sectorSize}

	if err := build.GetParams(true, true); err != nil {
		t.Fatalf("%+v", err)
	}

	ds := datastore.NewMapDatastore()

	dir, err := ioutil.TempDir("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	sb, err := sectorbuilder.TempSectorbuilderDir(dir, sectorSize, ds)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	cleanup := func() {
		sb.Destroy()
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	}

	defer cleanup()

	var wg sync.WaitGroup

	si1, err := sb.AcquireSectorId()
	if err != nil {
		t.Fatalf("%+v", err)
	}
	si2, err := sb.AcquireSectorId()
	if err != nil {
		t.Fatalf("%+v", err)
	}

	s1 := seal{sid: si1}
	s2 := seal{sid: si2}

	wg.Add(2)
	go s1.precommit(t, sb, 1, wg.Done)
	time.Sleep(100 * time.Millisecond)
	go s2.precommit(t, sb, 2, wg.Done)
	wg.Wait()

	wg.Add(2)
	go s1.commit(t, sb, wg.Done)
	go s2.commit(t, sb, wg.Done)
	wg.Wait()
}

func TestAcquireID(t *testing.T) {
	ds := datastore.NewMapDatastore()

	dir, err := ioutil.TempDir("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	sb, err := sectorbuilder.TempSectorbuilderDir(dir, sectorSize, ds)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	assertAcquire := func(expect uint64) {
		id, err := sb.AcquireSectorId()
		require.NoError(t, err)
		assert.Equal(t, expect, id)
	}

	assertAcquire(1)
	assertAcquire(2)
	assertAcquire(3)

	sb.Destroy()

	sb, err = sectorbuilder.TempSectorbuilderDir(dir, sectorSize, ds)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	assertAcquire(4)
	assertAcquire(5)
	assertAcquire(6)

	sb.Destroy()
	if err := os.RemoveAll(dir); err != nil {
		t.Error(err)
	}
}
