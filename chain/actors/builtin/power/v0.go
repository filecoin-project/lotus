package power

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
)

type v0State struct {
	power.State
	store adt.Store
}

func (s *v0State) TotalLocked() (abi.TokenAmount, error) {
	return s.TotalPledgeCollateral, nil
}

func (s *v0State) MinerNominalPowerMeetsConsensusMinimum(st adt.Store, a address.Address) (bool, error) {
	return s.State.MinerNominalPowerMeetsConsensusMinimum(st, a)
}
