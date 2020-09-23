package reward

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"

	miner1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	reward1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/reward"
	smoothing1 "github.com/filecoin-project/specs-actors/v2/actors/util/smoothing"
)

var _ State = (*state1)(nil)

type state1 struct {
	reward1.State
	store adt.Store
}

func (s *state1) ThisEpochReward() (abi.StoragePower, error) {
	return s.State.ThisEpochReward, nil
}

func (s *state1) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {
	return builtin.FilterEstimate{
		PositionEstimate: s.State.ThisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: s.State.ThisEpochRewardSmoothed.VelocityEstimate,
	}, nil
}

func (s *state1) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *state1) TotalStoragePowerReward() (abi.TokenAmount, error) {
	return s.State.TotalStoragePowerReward, nil
}

func (s *state1) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *state1) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *state1) CumsumBaseline() (abi.StoragePower, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state1) CumsumRealized() (abi.StoragePower, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state1) InitialPledgeForPower(qaPower abi.StoragePower, networkTotalPledge abi.TokenAmount, networkQAPower *builtin.FilterEstimate, circSupply abi.TokenAmount) (abi.TokenAmount, error) {
	return miner1.InitialPledgeForPower(
		qaPower,
		s.State.ThisEpochBaselinePower,
		s.State.ThisEpochRewardSmoothed,
		smoothing1.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		circSupply,
	), nil
}

func (s *state1) PreCommitDepositForPower(networkQAPower builtin.FilterEstimate, sectorWeight abi.StoragePower) (abi.TokenAmount, error) {
	return miner1.PreCommitDepositForPower(s.State.ThisEpochRewardSmoothed,
		smoothing1.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		sectorWeight), nil
}
