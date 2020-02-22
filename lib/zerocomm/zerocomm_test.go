package zerocomm

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-sectorbuilder"
)

func TestComms(t *testing.T) {
	var expPieceComms [levels - skip][32]byte

	{
		l2, err := sectorbuilder.GeneratePieceCommitment(bytes.NewReader(make([]byte, 127)), 127)
		if err != nil {
			return
		}
		expPieceComms[0] = l2
	}

	for i := 1; i < levels-2; i++ {
		var err error
		expPieceComms[i], err = sectorbuilder.GenerateDataCommitment(128<<i, []sectorbuilder.PublicPieceInfo{
			{
				Size:  127 << (i - 1),
				CommP: expPieceComms[i-1],
			},
			{
				Size:  127 << (i - 1),
				CommP: expPieceComms[i-1],
			},
		})
		if err != nil {
			panic(err)
		}
	}

	for i, comm := range expPieceComms {
		if string(comm[:]) != string(pieceComms[i][:]) {
			t.Errorf("zero commitment %d didn't match", i)
		}
	}

	for _, comm := range expPieceComms { // Could do codegen, but this is good enough
		fmt.Printf("%#v,\n", comm)
	}
}

func TestForSise(t *testing.T) {
	exp, err := sectorbuilder.GeneratePieceCommitment(bytes.NewReader(make([]byte, 1016)), 1016)
	if err != nil {
		return
	}

	actual := ForSize(1016)
	if string(exp[:]) != string(actual[:]) {
		t.Errorf("zero commitment didn't match")
	}
}
