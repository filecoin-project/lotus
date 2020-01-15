package storage

import (
	"bytes"
	"testing"

	"gotest.tools/assert"

	"github.com/filecoin-project/go-cbor-util"
)

func TestSectorInfoSelialization(t *testing.T) {
	si := &SectorInfo{
		State:    123,
		SectorID: 234,
		Nonce:    345,
		Pieces: []Piece{{
			DealID: 1234,
			Size:   5,
			CommP:  []byte{3},
		}},
		CommD: []byte{32, 4},
		CommR: nil,
		Proof: nil,
		Ticket: SealTicket{
			BlockHeight: 345,
			TicketBytes: []byte{87, 78, 7, 87},
		},
		PreCommitMessage: nil,
		Seed:             SealSeed{},
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
