package power

import (
	"bytes"
	"fmt"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin19 "github.com/filecoin-project/go-state-types/builtin"
	power19 "github.com/filecoin-project/go-state-types/builtin/v19/power"
	adt19 "github.com/filecoin-project/go-state-types/builtin/v19/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var _ State = (*state19)(nil)

func load19(store adt.Store, root cid.Cid) (State, error) {
	out := state19{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make19(store adt.Store) (State, error) {
	out := state19{store: store}

	s, err := power19.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state19 struct {
	power19.State
	store adt.Store
}

func (s *state19) TotalLocked() (abi.TokenAmount, error) {
	return s.TotalPledgeCollateral, nil
}

func (s *state19) TotalPower() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalRawBytePower,
		QualityAdjPower: s.TotalQualityAdjPower,
	}, nil
}

// Committed power to the network. Includes miners below the minimum threshold.
func (s *state19) TotalCommitted() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalBytesCommitted,
		QualityAdjPower: s.TotalQABytesCommitted,
	}, nil
}

func (s *state19) MinerPower(addr address.Address) (Claim, bool, error) {
	claims, err := s.claims()
	if err != nil {
		return Claim{}, false, err
	}
	var claim power19.Claim
	ok, err := claims.Get(abi.AddrKey(addr), &claim)
	if err != nil {
		return Claim{}, false, err
	}
	return Claim{
		RawBytePower:    claim.RawBytePower,
		QualityAdjPower: claim.QualityAdjPower,
	}, ok, nil
}

func (s *state19) MinerNominalPowerMeetsConsensusMinimum(a address.Address) (bool, error) {
	return s.State.MinerNominalPowerMeetsConsensusMinimum(s.store, a)
}

func (s *state19) TotalPowerSmoothed() (builtin.FilterEstimate, error) {
	return builtin.FilterEstimate(s.State.ThisEpochQAPowerSmoothed), nil
}

func (s *state19) MinerCounts() (uint64, uint64, error) {
	return uint64(s.State.MinerAboveMinPowerCount), uint64(s.State.MinerCount), nil
}

func (s *state19) RampStartEpoch() int64 {
	return s.State.RampStartEpoch
}

func (s *state19) RampDurationEpochs() uint64 {
	return s.State.RampDurationEpochs
}

func (s *state19) ListAllMiners() ([]address.Address, error) {
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

func (s *state19) CollectEligibleClaims(cacheInOut *builtin19.MapReduceCache) ([]builtin19.OwnedClaim, error) {

	return s.State.CollectEligibleClaims(s.store, cacheInOut)

}

func (s *state19) ForEachClaim(cb func(miner address.Address, claim Claim) error, onlyEligible bool) error {
	claims, err := s.claims()
	if err != nil {
		return err
	}

	var claim power19.Claim
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

func (s *state19) ClaimsChanged(other State) (bool, error) {
	other19, ok := other.(*state19)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.Claims.Equals(other19.State.Claims), nil
}

func (s *state19) SetTotalQualityAdjPower(p abi.StoragePower) error {
	s.State.TotalQualityAdjPower = p
	return nil
}

func (s *state19) SetTotalRawBytePower(p abi.StoragePower) error {
	s.State.TotalRawBytePower = p
	return nil
}

func (s *state19) SetThisEpochQualityAdjPower(p abi.StoragePower) error {
	s.State.ThisEpochQualityAdjPower = p
	return nil
}

func (s *state19) SetThisEpochRawBytePower(p abi.StoragePower) error {
	s.State.ThisEpochRawBytePower = p
	return nil
}

func (s *state19) GetState() interface{} {
	return &s.State
}

func (s *state19) claims() (adt.Map, error) {
	return adt19.AsMap(s.store, s.Claims, builtin19.DefaultHamtBitwidth)
}

func (s *state19) decodeClaim(val *cbg.Deferred) (Claim, error) {
	var ci power19.Claim
	if err := ci.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return Claim{}, err
	}
	return fromV19Claim(ci), nil
}

func fromV19Claim(v19 power19.Claim) Claim {
	return Claim{
		RawBytePower:    v19.RawBytePower,
		QualityAdjPower: v19.QualityAdjPower,
	}
}

func (s *state19) ActorKey() string {
	return manifest.PowerKey
}

func (s *state19) ActorVersion() actorstypes.Version {
	return actorstypes.Version19
}

func (s *state19) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
