package sectorbuilder_test

import (
	"io"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/ipfs/go-datastore"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/storage/sector"
)

func TestSealAndVerify(t *testing.T) {
	//t.Skip("this is slow")
	if err := build.GetParams(true); err != nil {
		t.Fatal(err)
	}

	dir, err := ioutil.TempDir("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	addr, err := address.NewFromString("t1tct3nfaw2q543xtybxcyw4deyxmfwkjk43u4t5y")
	if err != nil {
		t.Fatal(err)
	}

	sb, err := sectorbuilder.New(&sectorbuilder.SectorBuilderConfig{
		SectorSize:  1024,
		SealedDir:   dir,
		StagedDir:   dir,
		MetadataDir: dir,
		Miner:       addr,
	})
	if err != nil {
		t.Fatal(err)
	}

	r := io.LimitReader(rand.New(rand.NewSource(42)), 1016)

	if _, err := sb.AddPiece("foo", 1016, r); err != nil {
		t.Fatal(err)
	}

	store := sector.NewStore(sb, datastore.NewMapDatastore())
	store.Service()
	ssinfo := <-store.Incoming()

	ok, err := sectorbuilder.VerifySeal(1024, ssinfo.CommR[:], ssinfo.CommD[:], addr, ssinfo.Ticket.TicketBytes[:], ssinfo.SectorID, ssinfo.Proof)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("proof failed to validate")
	}
}
