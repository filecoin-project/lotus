package sealing

import (
	"bytes"
	"testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"gotest.tools/assert"

	cborutil "github.com/filecoin-project/go-cbor-util"
)

func TestSectorInfoSelialization(t *testing.T) {
	d := abi.DealID(1234)

	dummyCid := builtin.AccountActorCodeID

	si := &SectorInfo{
		State:    "stateful",
		SectorID: 234,
		Nonce:    345,
		Pieces: []Piece{{
			DealID: &d,
			Size:   5,
			CommP:  dummyCid,
		}},
		CommD: &dummyCid,
		CommR: nil,
		Proof: nil,
		Ticket: api.SealTicket{
			Epoch: 345,
			Value: []byte{87, 78, 7, 87},
		},
		PreCommitMessage: nil,
		Seed:             api.SealSeed{},
		CommitMessage:    nil,
		FaultReportMsg:   nil,
		LastErr:          "hi",
	}

	b, err := cborutil.Dump(si)
	if err != nil {
		t.Fatal(err)
	}

	var si2 SectorInfo
	if err := cborutil.ReadCborRPC(bytes.NewReader(b), &si); err != nil {
		return
	}

	assert.Equal(t, si.State, si2.State)
	assert.Equal(t, si.Nonce, si2.Nonce)
	assert.Equal(t, si.SectorID, si2.SectorID)

	assert.Equal(t, si.Pieces, si2.Pieces)
	assert.Equal(t, si.CommD, si2.CommD)
	assert.Equal(t, si.Ticket, si2.Ticket)

	assert.Equal(t, si, si2)

}
