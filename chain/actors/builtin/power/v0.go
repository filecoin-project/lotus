package power

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

type v0State struct {
	power.State
	store adt.Store
}

func (s *v0State) TotalLocked() (abi.TokenAmount, error) {
	return s.TotalPledgeCollateral, nil
}

func (s *v0State) TotalPower() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalRawBytePower,
		QualityAdjPower: s.TotalQualityAdjPower,
	}, nil
}

func (s *v0State) MinerPower(addr address.Address) (Claim, error) {
	claims, err := adt.AsMap(s.store, s.Claims)
	if err != nil {
		return Claim{}, err
	}
	var claim power.Claim
	if _, err := claims.Get(abi.AddrKey(addr), &claim); err != nil {
		return Claim{}, err
	}
	return Claim{
		RawBytePower:    claim.RawBytePower,
		QualityAdjPower: claim.QualityAdjPower,
	}, nil
}

func (s *v0State) MinerNominalPowerMeetsConsensusMinimum(a address.Address) (bool, error) {
	return s.State.MinerNominalPowerMeetsConsensusMinimum(s.store, a)
}
