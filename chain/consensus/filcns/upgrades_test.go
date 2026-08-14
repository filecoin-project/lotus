package filcns

import (
	"testing"

	"github.com/stretchr/testify/require"

	nv29 "github.com/filecoin-project/go-state-types/builtin/v19/migration"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

func TestSolsticeRewardMigrationConfig(t *testing.T) {
	t.Run("neutral bootstrap", func(t *testing.T) {
		params := buildconstants.NeutralSolsticeRewardBootstrapParams
		params.SWATimelockEpochs = 42

		config, err := solsticeRewardMigrationConfig(params)
		require.NoError(t, err)
		require.Equal(t, nv29.RewardMigrationConfig{
			SWATimelockEpochs: 42,
			SWAActor:          builtin.SystemActorAddr,
			Streams: []nv29.RewardMigrationStream{{
				ID: 1,
				Weight: nv29.RewardMigrationWeight{
					VStart: reward19.Denom,
					Floor:  reward19.Denom,
					Cap:    reward19.Denom,
				},
			}},
		}, config)
	})

	t.Run("zero duration rejects non-neutral consensus weight", func(t *testing.T) {
		params := buildconstants.NeutralSolsticeRewardBootstrapParams
		params.ConsensusWeight.Cap--

		_, err := solsticeRewardMigrationConfig(params)
		require.EqualError(t, err, "zero-duration Solstice bootstrap must have constant DENOM consensus weight and zero service weight")
	})

	t.Run("zero duration rejects service weight", func(t *testing.T) {
		params := buildconstants.NeutralSolsticeRewardBootstrapParams
		params.ServiceWeight.VStart = 1

		_, err := solsticeRewardMigrationConfig(params)
		require.EqualError(t, err, "zero-duration Solstice bootstrap must have constant DENOM consensus weight and zero service weight")
	})

	t.Run("negative duration", func(t *testing.T) {
		params := buildconstants.NeutralSolsticeRewardBootstrapParams
		params.ConsensusWeightRampDurationEpochs = -1

		_, err := solsticeRewardMigrationConfig(params)
		require.EqualError(t, err, "Solstice consensus weight ramp duration is negative: -1")
	})

	t.Run("split bootstrap", func(t *testing.T) {
		pct := reward19.Denom / 100
		params := buildconstants.SolsticeRewardBootstrapParams{
			SWATimelockEpochs:                 8,
			ConsensusWeightRampDurationEpochs: 81,
			ConsensusWeight: buildconstants.SolsticeRewardWeightParams{
				VStart: 95 * pct,
				Floor:  50 * pct,
				Cap:    95 * pct,
			},
			ServiceWeight: buildconstants.SolsticeRewardWeightParams{
				VStart: 5 * pct,
				Floor:  5 * pct,
				Cap:    10 * pct,
			},
			SWAActor:            builtin.SystemActorAddr,
			SRAActor:            builtin.SystemActorAddr,
			InitialOrchestrator: builtin.BurntFundsActorAddr,
		}
		rampTotal := params.ConsensusWeight.VStart - params.ConsensusWeight.Floor
		rampEpochs := uint64(params.ConsensusWeightRampDurationEpochs)
		slope := rampTotal / rampEpochs
		if rampTotal%rampEpochs != 0 {
			slope++
		}

		config, err := solsticeRewardMigrationConfig(params)
		require.NoError(t, err)
		require.Equal(t, nv29.RewardMigrationConfig{
			SWATimelockEpochs: params.SWATimelockEpochs,
			SWAActor:          params.SWAActor,
			Streams: []nv29.RewardMigrationStream{
				{
					ID: 1,
					Weight: nv29.RewardMigrationWeight{
						VStart: params.ConsensusWeight.VStart,
						Slope:  -int64(slope),
						Floor:  params.ConsensusWeight.Floor,
						Cap:    params.ConsensusWeight.Cap,
					},
				},
				{
					ID: 2,
					Weight: nv29.RewardMigrationWeight{
						VStart: params.ServiceWeight.VStart,
						Slope:  int64(slope),
						Floor:  params.ServiceWeight.Floor,
						Cap:    params.ServiceWeight.Cap,
					},
					Distribution: &reward19.DistributionInit{
						Writer: params.SRAActor,
						Shares: []reward19.RecipientShare{{
							Recipient: params.InitialOrchestrator,
							Share:     reward19.Denom,
						}},
					},
				},
			},
		}, config)
	})
}
