package zerocomm_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	commcid "github.com/filecoin-project/go-fil-commcid"
	abi "github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/zerocomm"
)

func TestComms(t *testing.T) {
	t.Skip("don't have enough ram") // no, but seriously, currently this needs like 3tb of /tmp

	var expPieceComms [zerocomm.Levels - zerocomm.Skip]cid.Cid

	{
		l2, err := ffiwrapper.GeneratePieceCIDFromFile(abi.RegisteredSealProof_StackedDrg2KiBV1, bytes.NewReader(make([]byte, 127)), 127)
		if err != nil {
			t.Fatal(err)
		}
		expPieceComms[0] = l2
	}

	for i := 1; i < zerocomm.Levels-2; i++ {
		var err error
		sz := abi.UnpaddedPieceSize(127 << uint(i))
		fmt.Println(i, sz)
		r := io.LimitReader(&NullReader{}, int64(sz))

		expPieceComms[i], err = ffiwrapper.GeneratePieceCIDFromFile(abi.RegisteredSealProof_StackedDrg2KiBV1, r, sz)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i, comm := range expPieceComms {
		c, err := commcid.CIDToPieceCommitmentV1(comm)
		if err != nil {
			t.Fatal(err)
		}
		if string(c) != string(zerocomm.PieceComms[i][:]) {
			t.Errorf("zero commitment %d didn't match", i)
		}
	}

	for _, comm := range expPieceComms { // Could do codegen, but this is good enough
		fmt.Printf("%#v,\n", comm)
	}
}

func TestCommsSmall(t *testing.T) {
	var expPieceComms [8]cid.Cid
	lvls := len(expPieceComms) + zerocomm.Skip

	{
		l2, err := ffiwrapper.GeneratePieceCIDFromFile(abi.RegisteredSealProof_StackedDrg2KiBV1, bytes.NewReader(make([]byte, 127)), 127)
		if err != nil {
			t.Fatal(err)
		}
		expPieceComms[0] = l2
	}

	for i := 1; i < lvls-2; i++ {
		var err error
		sz := abi.UnpaddedPieceSize(127 << uint(i))
		fmt.Println(i, sz)
		r := io.LimitReader(&NullReader{}, int64(sz))

		expPieceComms[i], err = ffiwrapper.GeneratePieceCIDFromFile(abi.RegisteredSealProof_StackedDrg2KiBV1, r, sz)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i, comm := range expPieceComms {
		c, err := commcid.CIDToPieceCommitmentV1(comm)
		if err != nil {
			t.Fatal(err)
		}
		if string(c) != string(zerocomm.PieceComms[i][:]) {
			t.Errorf("zero commitment %d didn't match", i)
		}
	}

	for _, comm := range expPieceComms { // Could do codegen, but this is good enough
		fmt.Printf("%#v,\n", comm)
	}
}

func TestForSise(t *testing.T) {
	exp, err := ffiwrapper.GeneratePieceCIDFromFile(abi.RegisteredSealProof_StackedDrg2KiBV1, bytes.NewReader(make([]byte, 1016)), 1016)
	if err != nil {
		return
	}

	actual := zerocomm.ZeroPieceCommitment(1016)
	if !exp.Equals(actual) {
		t.Errorf("zero commitment didn't match")
	}
}

type NullReader struct{}

func (NullReader) Read(out []byte) (int, error) {
	for i := range out {
		out[i] = 0
	}
	return len(out), nil
}
