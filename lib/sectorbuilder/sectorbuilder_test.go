package sectorbuilder_test

import (
	"io"
	"io/ioutil"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

const sectorSize = 1024

func TestSealAndVerify(t *testing.T) {
	t.Skip("this is slow")
	build.SectorSizes = []uint64{sectorSize}

	if err := build.GetParams(true); err != nil {
		t.Fatal(err)
	}

	dir, err := ioutil.TempDir("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	addr, err := address.NewFromString("t3vfxagwiegrywptkbmyohqqbfzd7xzbryjydmxso4hfhgsnv6apddyihltsbiikjf3lm7x2myiaxhuc77capq")
	if err != nil {
		t.Fatal(err)
	}

	cache := filepath.Join(dir, "cache")
	metadata := filepath.Join(dir, "meta")
	sealed := filepath.Join(dir, "sealed")
	staging := filepath.Join(dir, "staging")

	sb, err := sectorbuilder.New(&sectorbuilder.SectorBuilderConfig{
		SectorSize:  sectorSize,
		CacheDir:    cache,
		SealedDir:   sealed,
		StagedDir:   staging,
		MetadataDir: metadata,
		Miner:       addr,
	})
	if err != nil {
		t.Fatal(err)
	}

	dlen := sectorbuilder.UserBytesForSectorSize(sectorSize)

	r := io.LimitReader(rand.New(rand.NewSource(42)), int64(dlen))
	sid, err := sb.AddPiece("foo", dlen, r)
	if err != nil {
		t.Fatal(err)
	}

	ticket := sectorbuilder.SealTicket{
		BlockHeight: 5,
		TicketBytes: [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2},
	}

	pco, err := sb.SealPreCommit(sid, ticket)
	if err != nil {
		t.Fatal(err)
	}

	seed := sectorbuilder.SealSeed{
		BlockHeight: 15,
		TicketBytes: [32]byte{0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 45, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8},
	}

	sco, err := sb.SealCommit(sid, seed)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := sectorbuilder.VerifySeal(sectorSize, pco.CommR[:], pco.CommD[:], addr, ticket.TicketBytes[:], seed.TicketBytes[:], sid, sco.Proof)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("proof failed to validate")
	}
}
