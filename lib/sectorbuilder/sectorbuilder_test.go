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

	ffi "github.com/filecoin-project/filecoin-ffi"
	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

func init() {
	logging.SetLogLevel("*", "INFO")
}

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

	proof, err := sb.SealCommit(s.sid, s.ticket, seed, []sectorbuilder.PublicPieceInfo{s.ppi}, s.pco)
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

func post(t *testing.T, sb *sectorbuilder.SectorBuilder, seals ...seal) time.Time {
	cSeed := [32]byte{0, 9, 2, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9}

	ppi := make([]ffi.PublicSectorInfo, len(seals))
	for i, s := range seals {
		ppi[i] = ffi.PublicSectorInfo{
			SectorID: s.sid,
			CommR:    s.pco.CommR,
		}
	}

	ssi := sectorbuilder.NewSortedPublicSectorInfo(ppi)

	candndates, err := sb.GenerateEPostCandidates(ssi, cSeed, []uint64{})
	if err != nil {
		t.Fatalf("%+v", err)
	}

	genCandidates := time.Now()

	if len(candndates) != 1 {
		t.Fatal("expected 1 candidate")
	}

	postProof, err := sb.ComputeElectionPoSt(ssi, cSeed[:], candndates)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ok, err := sectorbuilder.VerifyElectionPost(context.TODO(), sb.SectorSize(), ssi, cSeed[:], postProof, candndates, sb.Miner)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if !ok {
		t.Fatal("bad post")
	}

	return genCandidates
}

func TestSealAndVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "info")

	build.SectorSizes = []uint64{sectorSize}

	if err := paramfetch.GetParams(sectorSize); err != nil {
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

	start := time.Now()

	s.precommit(t, sb, 1, func() {})

	precommit := time.Now()

	s.commit(t, sb, func() {})

	commit := time.Now()

	genCandidiates := post(t, sb, s)

	epost := time.Now()

	// Restart sectorbuilder, re-run post
	sb, err = sectorbuilder.TempSectorbuilderDir(dir, sectorSize, ds)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	post(t, sb, s)

	fmt.Printf("PreCommit: %s\n", precommit.Sub(start).String())
	fmt.Printf("Commit: %s\n", commit.Sub(precommit).String())
	fmt.Printf("GenCandidates: %s\n", genCandidiates.Sub(commit).String())
	fmt.Printf("EPoSt: %s\n", epost.Sub(genCandidiates).String())
}

func TestSealPoStNoCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "info")

	build.SectorSizes = []uint64{sectorSize}

	if err := paramfetch.GetParams(sectorSize); err != nil {
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

	start := time.Now()

	s.precommit(t, sb, 1, func() {})

	precommit := time.Now()

	// Restart sectorbuilder, re-run post
	sb, err = sectorbuilder.TempSectorbuilderDir(dir, sectorSize, ds)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	if err := sb.TrimCache(1); err != nil {
		t.Fatal(err)
	}

	genCandidiates := post(t, sb, s)

	epost := time.Now()

	fmt.Printf("PreCommit: %s\n", precommit.Sub(start).String())
	fmt.Printf("GenCandidates: %s\n", genCandidiates.Sub(precommit).String())
	fmt.Printf("EPoSt: %s\n", epost.Sub(genCandidiates).String())
}

func TestSealAndVerify2(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	if runtime.NumCPU() < 10 && os.Getenv("CI") == "" { // don't bother on slow hardware
		t.Skip("this is slow")
	}
	_ = os.Setenv("RUST_LOG", "info")

	build.SectorSizes = []uint64{sectorSize}

	if err := paramfetch.GetParams(sectorSize); err != nil {
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

	post(t, sb, s1, s2)
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

	sb, err = sectorbuilder.TempSectorbuilderDir(dir, sectorSize, ds)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	assertAcquire(4)
	assertAcquire(5)
	assertAcquire(6)

	if err := os.RemoveAll(dir); err != nil {
		t.Error(err)
	}
}
