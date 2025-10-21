package power

import (
	"bytes"
	"fmt"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin17 "github.com/filecoin-project/go-state-types/builtin"
	builtin18 "github.com/filecoin-project/go-state-types/builtin"
	power17 "github.com/filecoin-project/go-state-types/builtin/v17/power"
	adt17 "github.com/filecoin-project/go-state-types/builtin/v17/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var _ State = (*state17)(nil)

func load17(store adt.Store, root cid.Cid) (State, error) {
	out := state17{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make17(store adt.Store) (State, error) {
	out := state17{store: store}

	s, err := power17.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state17 struct {
	power17.State
	store adt.Store
}

func (s *state17) TotalLocked() (abi.TokenAmount, error) {
	return s.TotalPledgeCollateral, nil
}

func (s *state17) TotalPower() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalRawBytePower,
		QualityAdjPower: s.TotalQualityAdjPower,
	}, nil
}

// Committed power to the network. Includes miners below the minimum threshold.
func (s *state17) TotalCommitted() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalBytesCommitted,
		QualityAdjPower: s.TotalQABytesCommitted,
	}, nil
}

func (s *state17) MinerPower(addr address.Address) (Claim, bool, error) {
	claims, err := s.claims()
	if err != nil {
		return Claim{}, false, err
	}
	var claim power17.Claim
	ok, err := claims.Get(abi.AddrKey(addr), &claim)
	if err != nil {
		return Claim{}, false, err
	}
	return Claim{
		RawBytePower:    claim.RawBytePower,
		QualityAdjPower: claim.QualityAdjPower,
	}, ok, nil
}

func (s *state17) MinerNominalPowerMeetsConsensusMinimum(a address.Address) (bool, error) {
	return s.State.MinerNominalPowerMeetsConsensusMinimum(s.store, a)
}

func (s *state17) TotalPowerSmoothed() (builtin.FilterEstimate, error) {
	return builtin.FilterEstimate(s.State.ThisEpochQAPowerSmoothed), nil
}

func (s *state17) MinerCounts() (uint64, uint64, error) {
	return uint64(s.State.MinerAboveMinPowerCount), uint64(s.State.MinerCount), nil
}

func (s *state17) RampStartEpoch() int64 {
	return s.State.RampStartEpoch
}

func (s *state17) RampDurationEpochs() uint64 {
	return s.State.RampDurationEpochs
}

func (s *state17) ListAllMiners() ([]address.Address, error) {
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

func (s *state17) CollectEligibleClaims(cacheInOut *builtin18.MapReduceCache) ([]builtin18.OwnedClaim, error) {

	return s.State.CollectEligibleClaims(s.store, cacheInOut)

}

func (s *state17) ForEachClaim(cb func(miner address.Address, claim Claim) error, onlyEligible bool) error {
	claims, err := s.claims()
	if err != nil {
		return err
	}

	var claim power17.Claim
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

		eligible, err := s.State.ClaimMeetsConsensusMinimums(&claim)

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

func (s *state17) ClaimsChanged(other State) (bool, error) {
	other17, ok := other.(*state17)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.Claims.Equals(other17.State.Claims), nil
}

func (s *state17) SetTotalQualityAdjPower(p abi.StoragePower) error {
	s.State.TotalQualityAdjPower = p
	return nil
}

func (s *state17) SetTotalRawBytePower(p abi.StoragePower) error {
	s.State.TotalRawBytePower = p
	return nil
}

func (s *state17) SetThisEpochQualityAdjPower(p abi.StoragePower) error {
	s.State.ThisEpochQualityAdjPower = p
	return nil
}

func (s *state17) SetThisEpochRawBytePower(p abi.StoragePower) error {
	s.State.ThisEpochRawBytePower = p
	return nil
}

func (s *state17) GetState() interface{} {
	return &s.State
}

func (s *state17) claims() (adt.Map, error) {
	return adt17.AsMap(s.store, s.Claims, builtin17.DefaultHamtBitwidth)
}

func (s *state17) decodeClaim(val *cbg.Deferred) (Claim, error) {
	var ci power17.Claim
	if err := ci.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return Claim{}, err
	}
	return fromV17Claim(ci), nil
}

func fromV17Claim(v17 power17.Claim) Claim {
	return Claim{
		RawBytePower:    v17.RawBytePower,
		QualityAdjPower: v17.QualityAdjPower,
	}
}

func (s *state17) ActorKey() string {
	return manifest.PowerKey
}

func (s *state17) ActorVersion() actorstypes.Version {
	return actorstypes.Version17
}

func (s *state17) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
