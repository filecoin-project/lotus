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
	power18 "github.com/filecoin-project/go-state-types/builtin/v18/power"
	adt18 "github.com/filecoin-project/go-state-types/builtin/v18/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var _ State = (*state18)(nil)

func load18(store adt.Store, root cid.Cid) (State, error) {
	out := state18{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make18(store adt.Store) (State, error) {
	out := state18{store: store}

	s, err := power18.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state18 struct {
	power18.State
	store adt.Store
}

func (s *state18) TotalLocked() (abi.TokenAmount, error) {
	return s.TotalPledgeCollateral, nil
}

func (s *state18) TotalPower() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalRawBytePower,
		QualityAdjPower: s.TotalQualityAdjPower,
	}, nil
}

// Committed power to the network. Includes miners below the minimum threshold.
func (s *state18) TotalCommitted() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalBytesCommitted,
		QualityAdjPower: s.TotalQABytesCommitted,
	}, nil
}

func (s *state18) MinerPower(addr address.Address) (Claim, bool, error) {
	claims, err := s.claims()
	if err != nil {
		return Claim{}, false, err
	}
	var claim power18.Claim
	ok, err := claims.Get(abi.AddrKey(addr), &claim)
	if err != nil {
		return Claim{}, false, err
	}
	return Claim{
		RawBytePower:    claim.RawBytePower,
		QualityAdjPower: claim.QualityAdjPower,
	}, ok, nil
}

func (s *state18) MinerNominalPowerMeetsConsensusMinimum(a address.Address) (bool, error) {
	return s.State.MinerNominalPowerMeetsConsensusMinimum(s.store, a)
}

func (s *state18) TotalPowerSmoothed() (builtin.FilterEstimate, error) {
	return builtin.FilterEstimate(s.State.ThisEpochQAPowerSmoothed), nil
}

func (s *state18) MinerCounts() (uint64, uint64, error) {
	return uint64(s.State.MinerAboveMinPowerCount), uint64(s.State.MinerCount), nil
}

func (s *state18) RampStartEpoch() int64 {
	return s.State.RampStartEpoch
}

func (s *state18) RampDurationEpochs() uint64 {
	return s.State.RampDurationEpochs
}

func (s *state18) ListAllMiners() ([]address.Address, error) {
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

func (s *state18) CollectEligibleClaims(cacheInOut *builtin18.MapReduceCache) ([]builtin18.OwnedClaim, error) {

	return s.State.CollectEligibleClaims(s.store, cacheInOut)

}

func (s *state18) ForEachClaim(cb func(miner address.Address, claim Claim) error, onlyEligible bool) error {
	claims, err := s.claims()
	if err != nil {
		return err
	}

	var claim power18.Claim
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

func (s *state18) ClaimsChanged(other State) (bool, error) {
	other18, ok := other.(*state18)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.Claims.Equals(other18.State.Claims), nil
}

func (s *state18) SetTotalQualityAdjPower(p abi.StoragePower) error {
	s.State.TotalQualityAdjPower = p
	return nil
}

func (s *state18) SetTotalRawBytePower(p abi.StoragePower) error {
	s.State.TotalRawBytePower = p
	return nil
}

func (s *state18) SetThisEpochQualityAdjPower(p abi.StoragePower) error {
	s.State.ThisEpochQualityAdjPower = p
	return nil
}

func (s *state18) SetThisEpochRawBytePower(p abi.StoragePower) error {
	s.State.ThisEpochRawBytePower = p
	return nil
}

func (s *state18) GetState() interface{} {
	return &s.State
}

func (s *state18) claims() (adt.Map, error) {
	return adt18.AsMap(s.store, s.Claims, builtin18.DefaultHamtBitwidth)
}

func (s *state18) decodeClaim(val *cbg.Deferred) (Claim, error) {
	var ci power18.Claim
	if err := ci.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return Claim{}, err
	}
	return fromV18Claim(ci), nil
}

func fromV18Claim(v18 power18.Claim) Claim {
	return Claim{
		RawBytePower:    v18.RawBytePower,
		QualityAdjPower: v18.QualityAdjPower,
	}
}

func (s *state18) ActorKey() string {
	return manifest.PowerKey
}

func (s *state18) ActorVersion() actorstypes.Version {
	return actorstypes.Version18
}

func (s *state18) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
