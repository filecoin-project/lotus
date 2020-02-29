package zerocomm

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-sectorbuilder"
	abi "github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/lib/nullreader"
)

func TestComms(t *testing.T) {
	t.Skip("don't have enough ram") // no, but seriously, currently this needs like 3tb of /tmp

	var expPieceComms [levels - skip]cid.Cid

	{
		l2, err := sectorbuilder.GeneratePieceCIDFromFile(abi.RegisteredProof_StackedDRG2KiBPoSt, bytes.NewReader(make([]byte, 127)), 127)
		if err != nil {
			t.Fatal(err)
		}
		expPieceComms[0] = l2
	}

	for i := 1; i < levels-2; i++ {
		var err error
		sz := abi.UnpaddedPieceSize(127 << i)
		fmt.Println(i, sz)
		r := io.LimitReader(&nullreader.Reader{}, int64(sz))

		expPieceComms[i], err = sectorbuilder.GeneratePieceCIDFromFile(abi.RegisteredProof_StackedDRG2KiBPoSt, r, sz)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i, comm := range expPieceComms {
		c, err := commcid.CIDToPieceCommitmentV1(comm)
		if err != nil {
			t.Fatal(err)
		}
		if string(c) != string(pieceComms[i][:]) {
			t.Errorf("zero commitment %d didn't match", i)
		}
	}

	for _, comm := range expPieceComms { // Could do codegen, but this is good enough
		fmt.Printf("%#v,\n", comm)
	}
}

func TestCommsSmall(t *testing.T) {
	var expPieceComms [8]cid.Cid
	lvls := len(expPieceComms) + skip

	{
		l2, err := sectorbuilder.GeneratePieceCIDFromFile(abi.RegisteredProof_StackedDRG2KiBPoSt, bytes.NewReader(make([]byte, 127)), 127)
		if err != nil {
			t.Fatal(err)
		}
		expPieceComms[0] = l2
	}

	for i := 1; i < lvls-2; i++ {
		var err error
		sz := abi.UnpaddedPieceSize(127 << i)
		fmt.Println(i, sz)
		r := io.LimitReader(&nullreader.Reader{}, int64(sz))

		expPieceComms[i], err = sectorbuilder.GeneratePieceCIDFromFile(abi.RegisteredProof_StackedDRG2KiBPoSt, r, sz)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i, comm := range expPieceComms {
		c, err := commcid.CIDToPieceCommitmentV1(comm)
		if err != nil {
			t.Fatal(err)
		}
		if string(c) != string(pieceComms[i][:]) {
			t.Errorf("zero commitment %d didn't match", i)
		}
	}

	for _, comm := range expPieceComms { // Could do codegen, but this is good enough
		fmt.Printf("%#v,\n", comm)
	}
}

func TestForSise(t *testing.T) {
	exp, err := sectorbuilder.GeneratePieceCIDFromFile(abi.RegisteredProof_StackedDRG2KiBPoSt, bytes.NewReader(make([]byte, 1016)), 1016)
	if err != nil {
		return
	}

	actual := ForSize(1016)
	if !exp.Equals(actual) {
		t.Errorf("zero commitment didn't match")
	}
}
