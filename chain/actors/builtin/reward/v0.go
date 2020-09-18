package reward

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

type v0State struct {
	reward.State
	store adt.Store
}

func (s *v0State) ThisEpochReward() (abi.StoragePower, error) {
	return s.State.ThisEpochReward, nil
}

func (s *v0State) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {
	return *s.State.ThisEpochRewardSmoothed, nil
}

func (s *v0State) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *v0State) TotalStoragePowerReward() (abi.TokenAmount, error) {
	return s.State.TotalMined, nil
}

func (s *v0State) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *v0State) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *v0State) CumsumBaseline() (abi.StoragePower, error) {
	return s.State.CumsumBaseline, nil
}

func (s *v0State) CumsumRealized() (abi.StoragePower, error) {
	return s.State.CumsumBaseline, nil
}
