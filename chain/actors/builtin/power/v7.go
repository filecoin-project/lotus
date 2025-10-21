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
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"
	power7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/power"
	adt7 "github.com/filecoin-project/specs-actors/v7/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var _ State = (*state7)(nil)

func load7(store adt.Store, root cid.Cid) (State, error) {
	out := state7{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make7(store adt.Store) (State, error) {
	out := state7{store: store}

	s, err := power7.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state7 struct {
	power7.State
	store adt.Store
}

func (s *state7) TotalLocked() (abi.TokenAmount, error) {
	return s.TotalPledgeCollateral, nil
}

func (s *state7) TotalPower() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalRawBytePower,
		QualityAdjPower: s.TotalQualityAdjPower,
	}, nil
}

// Committed power to the network. Includes miners below the minimum threshold.
func (s *state7) TotalCommitted() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalBytesCommitted,
		QualityAdjPower: s.TotalQABytesCommitted,
	}, nil
}

func (s *state7) MinerPower(addr address.Address) (Claim, bool, error) {
	claims, err := s.claims()
	if err != nil {
		return Claim{}, false, err
	}
	var claim power7.Claim
	ok, err := claims.Get(abi.AddrKey(addr), &claim)
	if err != nil {
		return Claim{}, false, err
	}
	return Claim{
		RawBytePower:    claim.RawBytePower,
		QualityAdjPower: claim.QualityAdjPower,
	}, ok, nil
}

func (s *state7) MinerNominalPowerMeetsConsensusMinimum(a address.Address) (bool, error) {
	return s.State.MinerNominalPowerMeetsConsensusMinimum(s.store, a)
}

func (s *state7) TotalPowerSmoothed() (builtin.FilterEstimate, error) {
	return builtin.FilterEstimate(s.State.ThisEpochQAPowerSmoothed), nil
}

func (s *state7) MinerCounts() (uint64, uint64, error) {
	return uint64(s.State.MinerAboveMinPowerCount), uint64(s.State.MinerCount), nil
}

func (s *state7) RampStartEpoch() int64 {
	return 0
}

func (s *state7) RampDurationEpochs() uint64 {
	return 0
}

func (s *state7) ListAllMiners() ([]address.Address, error) {
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

func (s *state7) CollectEligibleClaims(cacheInOut *builtin18.MapReduceCache) ([]builtin18.OwnedClaim, error) {

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

func (s *state7) ForEachClaim(cb func(miner address.Address, claim Claim) error, onlyEligible bool) error {
	claims, err := s.claims()
	if err != nil {
		return err
	}

	var claim power7.Claim
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

func (s *state7) ClaimsChanged(other State) (bool, error) {
	other7, ok := other.(*state7)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.Claims.Equals(other7.State.Claims), nil
}

func (s *state7) SetTotalQualityAdjPower(p abi.StoragePower) error {
	s.State.TotalQualityAdjPower = p
	return nil
}

func (s *state7) SetTotalRawBytePower(p abi.StoragePower) error {
	s.State.TotalRawBytePower = p
	return nil
}

func (s *state7) SetThisEpochQualityAdjPower(p abi.StoragePower) error {
	s.State.ThisEpochQualityAdjPower = p
	return nil
}

func (s *state7) SetThisEpochRawBytePower(p abi.StoragePower) error {
	s.State.ThisEpochRawBytePower = p
	return nil
}

func (s *state7) GetState() interface{} {
	return &s.State
}

func (s *state7) claims() (adt.Map, error) {
	return adt7.AsMap(s.store, s.Claims, builtin7.DefaultHamtBitwidth)
}

func (s *state7) decodeClaim(val *cbg.Deferred) (Claim, error) {
	var ci power7.Claim
	if err := ci.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return Claim{}, err
	}
	return fromV7Claim(ci), nil
}

func fromV7Claim(v7 power7.Claim) Claim {
	return Claim{
		RawBytePower:    v7.RawBytePower,
		QualityAdjPower: v7.QualityAdjPower,
	}
}

func (s *state7) ActorKey() string {
	return manifest.PowerKey
}

func (s *state7) ActorVersion() actorstypes.Version {
	return actorstypes.Version7
}

func (s *state7) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
