package storage

import (
	sectorbuilder "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFPostDecodeMessage(t *testing.T) {
	scandidates := []sectorbuilder.Candidate{{
		SectorID:             1,
		PartialTicket:        [32]byte{1},
		Ticket:               [32]byte{1},
		SectorChallengeIndex: 0,
	}, {
		SectorID:             2,
		PartialTicket:        [32]byte{1, 2},
		Ticket:               [32]byte{1, 2},
		SectorChallengeIndex: 1,
	},
	}
	candidates := make([]types.EPostTicket, len(scandidates))
	for i, sc := range scandidates {
		candidates[i] = types.EPostTicket{
			Partial:        sc.PartialTicket[:],
			SectorID:       sc.SectorID,
			ChallengeIndex: sc.SectorChallengeIndex,
		}
	}
	params := &actors.SubmitFallbackPoStParams{
		Proof:      []byte{0, 1, 2, 3},
		Candidates: candidates,
	}
	enc, err := actors.SerializeParams(params)
	assert.NoError(t, err)
	var p actors.SubmitFallbackPoStParams
	_ = vm.DecodeParams(enc, &p)
	assert.Equal(t, 2, len(p.Candidates))
	assert.Equal(t, candidates[0].Partial, p.Candidates[0].Partial)
	assert.Equal(t, candidates[1].Partial, p.Candidates[1].Partial)

	assert.Equal(t, scandidates[1].PartialTicket[:], p.Candidates[1].Partial)

	//Expected :[]byte{0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}
	//Actual   :[]byte{0x1, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}
	assert.Equal(t, scandidates[0].PartialTicket[:], p.Candidates[0].Partial)

}
