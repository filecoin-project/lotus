package sectorbuilder

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-lotus/chain/address"
)

func TestSealAndVerify(t *testing.T) {
	t.Skip("this is slow")
	dir, err := ioutil.TempDir("", "sbtest")
	if err != nil {
		t.Fatal(err)
	}

	addr, err := address.NewFromString("t1tct3nfaw2q543xtybxcyw4deyxmfwkjk43u4t5y")
	if err != nil {
		t.Fatal(err)
	}

	sb, err := New(&SectorBuilderConfig{
		SectorSize:  1024,
		SealedDir:   dir,
		StagedDir:   dir,
		MetadataDir: dir,
		Miner:       addr,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sb.Run(ctx)

	fi, err := ioutil.TempFile("", "sbtestfi")
	if err != nil {
		t.Fatal(err)
	}
	defer fi.Close()

	io.CopyN(fi, rand.New(rand.NewSource(42)), 1016)

	if _, err := sb.AddPiece("foo", 1016, fi.Name()); err != nil {
		t.Fatal(err)
	}

	ssinfo := <-sb.SealedSectorChan()
	fmt.Println("sector sealed...")

	ok, err := VerifySeal(1024, ssinfo.CommR[:], ssinfo.CommD[:], ssinfo.CommRStar[:], addr, ssinfo.SectorID, ssinfo.Proof)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("proof failed to validate")
	}
}
