package sealing

import (
	"bytes"
	"testing"

	"gotest.tools/assert"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
)

func TestSectorInfoSelialization(t *testing.T) {
	d := abi.DealID(1234)

	dealInfo := DealInfo{
		DealID: d,
		DealSchedule: DealSchedule{
			StartEpoch: 0,
			EndEpoch:   100,
		},
	}

	dummyCid := builtin.AccountActorCodeID

	si := &SectorInfo{
		State:        "stateful",
		SectorNumber: 234,
		Pieces: []Piece{{
			Piece: abi.PieceInfo{
				Size:     5,
				PieceCID: dummyCid,
			},
			DealInfo: &dealInfo,
		}},
		CommD:            &dummyCid,
		CommR:            nil,
		Proof:            nil,
		TicketValue:      []byte{87, 78, 7, 87},
		TicketEpoch:      345,
		PreCommitMessage: nil,
		SeedValue:        []byte{},
		SeedEpoch:        0,
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
	assert.Equal(t, si.SectorNumber, si2.SectorNumber)

	assert.Equal(t, si.Pieces, si2.Pieces)
	assert.Equal(t, si.CommD, si2.CommD)
	assert.Equal(t, si.TicketValue, si2.TicketValue)
	assert.Equal(t, si.TicketEpoch, si2.TicketEpoch)

	assert.Equal(t, si, si2)

}
