package reward

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

type state0 struct {
	reward.State
	store adt.Store
}

func (s *state0) ThisEpochReward() (abi.StoragePower, error) {
	return s.State.ThisEpochReward, nil
}

func (s *state0) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {
	return *s.State.ThisEpochRewardSmoothed, nil
}

func (s *state0) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *state0) TotalStoragePowerReward() (abi.TokenAmount, error) {
	return s.State.TotalMined, nil
}

func (s *state0) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *state0) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *state0) CumsumBaseline() (abi.StoragePower, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state0) CumsumRealized() (abi.StoragePower, error) {
	return s.State.CumsumBaseline, nil
}
