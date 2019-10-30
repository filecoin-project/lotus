package sectorbuilder_test

import (
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-datastore"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
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
		CacheDir:cache,
		SealedDir:   sealed,
		StagedDir:   staging,
		MetadataDir: metadata,
		Miner:       addr,
	})
	if err != nil {
		t.Fatal(err)
	}

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

	ok, err := sectorbuilder.VerifySeal(sectorSize, ssinfo.CommR[:], ssinfo.CommD[:], addr, ssinfo.Ticket.TicketBytes[:], ssinfo.SectorID, ssinfo.Proof)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("proof failed to validate")
	}
}
