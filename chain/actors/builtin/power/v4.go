package power

import (
	"bytes"
	"fmt"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin18 "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/manifest"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	power4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/power"
	adt4 "github.com/filecoin-project/specs-actors/v4/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var _ State = (*state4)(nil)

func load4(store adt.Store, root cid.Cid) (State, error) {
	out := state4{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make4(store adt.Store) (State, error) {
	out := state4{store: store}

	s, err := power4.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state4 struct {
	power4.State
	store adt.Store
}

func (s *state4) TotalLocked() (abi.TokenAmount, error) {
	return s.TotalPledgeCollateral, nil
}

func (s *state4) TotalPower() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalRawBytePower,
		QualityAdjPower: s.TotalQualityAdjPower,
	}, nil
}

// Committed power to the network. Includes miners below the minimum threshold.
func (s *state4) TotalCommitted() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalBytesCommitted,
		QualityAdjPower: s.TotalQABytesCommitted,
	}, nil
}

func (s *state4) MinerPower(addr address.Address) (Claim, bool, error) {
	claims, err := s.claims()
	if err != nil {
		return Claim{}, false, err
	}
	var claim power4.Claim
	ok, err := claims.Get(abi.AddrKey(addr), &claim)
	if err != nil {
		return Claim{}, false, err
	}
	return Claim{
		RawBytePower:    claim.RawBytePower,
		QualityAdjPower: claim.QualityAdjPower,
	}, ok, nil
}

func (s *state4) MinerNominalPowerMeetsConsensusMinimum(a address.Address) (bool, error) {
	return s.State.MinerNominalPowerMeetsConsensusMinimum(s.store, a)
}

func (s *state4) TotalPowerSmoothed() (builtin.FilterEstimate, error) {
	return builtin.FilterEstimate(s.State.ThisEpochQAPowerSmoothed), nil
}

func (s *state4) MinerCounts() (uint64, uint64, error) {
	return uint64(s.State.MinerAboveMinPowerCount), uint64(s.State.MinerCount), nil
}

func (s *state4) RampStartEpoch() int64 {
	return 0
}

func (s *state4) RampDurationEpochs() uint64 {
	return 0
}

func (s *state4) ListAllMiners() ([]address.Address, error) {
	claims, err := s.claims()
	if err != nil {
		return nil, err
	}

	var miners []address.Address
	err = claims.ForEach(nil, func(k string) error {
		a, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}
		miners = append(miners, a)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return miners, nil
}

func (s *state4) CollectEligibleClaims(cacheInOut *builtin18.MapReduceCache) ([]builtin18.OwnedClaim, error) {

	var res []builtin18.OwnedClaim
	err := s.ForEachClaim(func(miner address.Address, claim Claim) error {
		res = append(res, builtin18.OwnedClaim{
			Address:         miner,
			RawBytePower:    claim.RawBytePower,
			QualityAdjPower: claim.QualityAdjPower,
		})
		return nil
	}, true)
	if err != nil {
		return nil, fmt.Errorf("collecting claims: %w", err)
	}
	return res, nil

}

func (s *state4) ForEachClaim(cb func(miner address.Address, claim Claim) error, onlyEligible bool) error {
	claims, err := s.claims()
	if err != nil {
		return err
	}

	var claim power4.Claim
	return claims.ForEach(&claim, func(k string) error {
		a, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}
		if !onlyEligible {
			return cb(a, Claim{
				RawBytePower:    claim.RawBytePower,
				QualityAdjPower: claim.QualityAdjPower,
			})
		}

		//slow path
		eligible, err := s.State.MinerNominalPowerMeetsConsensusMinimum(s.store, a)

		if err != nil {
			return fmt.Errorf("checking consensus minimums: %w", err)
		}
		if eligible {
			return cb(a, Claim{
				RawBytePower:    claim.RawBytePower,
				QualityAdjPower: claim.QualityAdjPower,
			})
		}
		return nil
	})
}

func (s *state4) ClaimsChanged(other State) (bool, error) {
	other4, ok := other.(*state4)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.Claims.Equals(other4.State.Claims), nil
}

func (s *state4) SetTotalQualityAdjPower(p abi.StoragePower) error {
	s.State.TotalQualityAdjPower = p
	return nil
}

func (s *state4) SetTotalRawBytePower(p abi.StoragePower) error {
	s.State.TotalRawBytePower = p
	return nil
}

func (s *state4) SetThisEpochQualityAdjPower(p abi.StoragePower) error {
	s.State.ThisEpochQualityAdjPower = p
	return nil
}

func (s *state4) SetThisEpochRawBytePower(p abi.StoragePower) error {
	s.State.ThisEpochRawBytePower = p
	return nil
}

func (s *state4) GetState() interface{} {
	return &s.State
}

func (s *state4) claims() (adt.Map, error) {
	return adt4.AsMap(s.store, s.Claims, builtin4.DefaultHamtBitwidth)
}

func (s *state4) decodeClaim(val *cbg.Deferred) (Claim, error) {
	var ci power4.Claim
	if err := ci.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return Claim{}, err
	}
	return fromV4Claim(ci), nil
}

func fromV4Claim(v4 power4.Claim) Claim {
	return Claim{
		RawBytePower:    v4.RawBytePower,
		QualityAdjPower: v4.QualityAdjPower,
	}
}

func (s *state4) ActorKey() string {
	return manifest.PowerKey
}

func (s *state4) ActorVersion() actorstypes.Version {
	return actorstypes.Version4
}

func (s *state4) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
