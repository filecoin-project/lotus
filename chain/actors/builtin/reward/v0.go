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

func (s *v0State) RewardSmoothed() (builtin.FilterEstimate, error) {
	return *s.State.ThisEpochRewardSmoothed, nil
}

func (s *v0State) TotalStoragePowerReward() abi.TokenAmount {
	return s.State.TotalMined
}
