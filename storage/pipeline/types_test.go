package sealing

import (
	"bytes"
	"testing"

	"github.com/ipfs/go-cid"
	"gotest.tools/assert"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	tutils "github.com/filecoin-project/specs-actors/v2/support/testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
)

func TestSectorInfoSerialization(t *testing.T) {
	d := abi.DealID(1234)

	dummyCid, err := cid.Parse("bafkqaaa")
	if err != nil {
		t.Fatal(err)
	}

	dealInfo := piece.PieceDealInfo{
		DealID: d,
		DealSchedule: piece.DealSchedule{
			StartEpoch: 0,
			EndEpoch:   100,
		},
		DealProposal: &markettypes.DealProposal{
			PieceCID:             dummyCid,
			PieceSize:            5,
			Client:               tutils.NewActorAddr(t, "client"),
			Provider:             tutils.NewActorAddr(t, "provider"),
			StoragePricePerEpoch: abi.NewTokenAmount(10),
			ProviderCollateral:   abi.NewTokenAmount(20),
			ClientCollateral:     abi.NewTokenAmount(15),
		},
	}

	si := &SectorInfo{
		State:        "stateful",
		SectorNumber: 234,
		Pieces: []SafeSectorPiece{{real: api.SectorPiece{
			Piece: abi.PieceInfo{
				Size:     5,
				PieceCID: dummyCid,
			},
			DealInfo: &dealInfo,
		}}},
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
	if err := cborutil.ReadCborRPC(bytes.NewReader(b), &si2); err != nil {
		t.Fatal(err)
		return
	}

	assert.Equal(t, si.State, si2.State)
	assert.Equal(t, si.SectorNumber, si2.SectorNumber)

	assert.Equal(t, si.Pieces[0].Impl().DealID, si2.Pieces[0].Impl().DealID)
	assert.Equal(t, si.Pieces[0].Impl().DealProposal.PieceCID, si2.Pieces[0].Impl().DealProposal.PieceCID)
	assert.Equal(t, *si.CommD, *si2.CommD)
	assert.DeepEqual(t, si.TicketValue, si2.TicketValue)
	assert.Equal(t, si.TicketEpoch, si2.TicketEpoch)
	assert.Equal(t, si.TicketEpoch, si2.TicketEpoch)
}
