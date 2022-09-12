package bcast_test

import (
	"crypto/rand"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestSimpleDelivery(t *testing.T) {
}

func newBlock(t *testing.T, epoch abi.ChainEpoch) *types.BlockMsg {
	proof := make([]byte, 10)
	_, err := rand.Read(proof)
	if err != err {
		t.Fatal(err)
	}
	bh := &types.BlockHeader{
		Ticket: &types.Ticket{
			VRFProof: []byte("vrf proof0000000vrf proof0000000"),
		},
		Height: 85919298723,
	}
	return &types.BlockMsg{
		Header: bh,
	}
}
