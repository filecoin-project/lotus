package policy

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	verifreg0 "github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	verifreg2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"
)

func TestSupportedProofTypes(t *testing.T) {
	var oldTypes []abi.RegisteredSealProof
	for t := range miner0.SupportedProofTypes {
		oldTypes = append(oldTypes, t)
	}
	t.Cleanup(func() {
		SetSupportedProofTypes(oldTypes...)
	})

	SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	require.EqualValues(t,
		miner0.SupportedProofTypes,
		map[abi.RegisteredSealProof]struct{}{
			abi.RegisteredSealProof_StackedDrg2KiBV1: {},
		},
	)
	AddSupportedProofTypes(abi.RegisteredSealProof_StackedDrg8MiBV1)
	require.EqualValues(t,
		miner0.SupportedProofTypes,
		map[abi.RegisteredSealProof]struct{}{
			abi.RegisteredSealProof_StackedDrg2KiBV1: {},
			abi.RegisteredSealProof_StackedDrg8MiBV1: {},
		},
	)
}

// Tests assumptions about policies being the same between actor versions.
func TestAssumptions(t *testing.T) {
	require.EqualValues(t, miner0.SupportedProofTypes, miner2.SupportedProofTypes)
	require.Equal(t, miner0.PreCommitChallengeDelay, miner2.PreCommitChallengeDelay)
	require.Equal(t, miner0.ChainFinality, miner2.ChainFinality)
	require.Equal(t, miner0.WPoStChallengeWindow, miner2.WPoStChallengeWindow)
	require.Equal(t, miner0.WPoStProvingPeriod, miner2.WPoStProvingPeriod)
	require.Equal(t, miner0.WPoStPeriodDeadlines, miner2.WPoStPeriodDeadlines)
	require.True(t, verifreg0.MinVerifiedDealSize.Equals(verifreg2.MinVerifiedDealSize))
}
