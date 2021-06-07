package power

import (
	"bytes"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"

	power5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/power"
	adt5 "github.com/filecoin-project/specs-actors/v5/actors/util/adt"
)

var _ State = (*state5)(nil)

func load5(store adt.Store, root cid.Cid) (State, error) {
	out := state5{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make5(store adt.Store) (State, error) {
	out := state5{store: store}

	s, err := power5.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state5 struct {
	power5.State
	store adt.Store
}

func (s *state5) TotalLocked() (abi.TokenAmount, error) {
	return s.TotalPledgeCollateral, nil
}

func (s *state5) TotalPower() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalRawBytePower,
		QualityAdjPower: s.TotalQualityAdjPower,
	}, nil
}

// Committed power to the network. Includes miners below the minimum threshold.
func (s *state5) TotalCommitted() (Claim, error) {
	return Claim{
		RawBytePower:    s.TotalBytesCommitted,
		QualityAdjPower: s.TotalQABytesCommitted,
	}, nil
}

func (s *state5) MinerPower(addr address.Address) (Claim, bool, error) {
	claims, err := s.claims()
	if err != nil {
		return Claim{}, false, err
	}
	var claim power5.Claim
	ok, err := claims.Get(abi.AddrKey(addr), &claim)
	if err != nil {
		return Claim{}, false, err
	}
	return Claim{
		RawBytePower:    claim.RawBytePower,
		QualityAdjPower: claim.QualityAdjPower,
	}, ok, nil
}

func (s *state5) MinerNominalPowerMeetsConsensusMinimum(a address.Address) (bool, error) {
	return s.State.MinerNominalPowerMeetsConsensusMinimum(s.store, a)
}

func (s *state5) TotalPowerSmoothed() (builtin.FilterEstimate, error) {
	return builtin.FromV5FilterEstimate(s.State.ThisEpochQAPowerSmoothed), nil
}

func (s *state5) MinerCounts() (uint64, uint64, error) {
	return uint64(s.State.MinerAboveMinPowerCount), uint64(s.State.MinerCount), nil
}

func (s *state5) ListAllMiners() ([]address.Address, error) {
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

func (s *state5) ForEachClaim(cb func(miner address.Address, claim Claim) error) error {
	claims, err := s.claims()
	if err != nil {
		return err
	}

	var claim power5.Claim
	return claims.ForEach(&claim, func(k string) error {
		a, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}
		return cb(a, Claim{
			RawBytePower:    claim.RawBytePower,
			QualityAdjPower: claim.QualityAdjPower,
		})
	})
}

func (s *state5) ClaimsChanged(other State) (bool, error) {
	other5, ok := other.(*state5)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.Claims.Equals(other5.State.Claims), nil
}

func (s *state5) SetTotalQualityAdjPower(p abi.StoragePower) error {
	s.State.TotalQualityAdjPower = p
	return nil
}

func (s *state5) SetTotalRawBytePower(p abi.StoragePower) error {
	s.State.TotalRawBytePower = p
	return nil
}

func (s *state5) SetThisEpochQualityAdjPower(p abi.StoragePower) error {
	s.State.ThisEpochQualityAdjPower = p
	return nil
}

func (s *state5) SetThisEpochRawBytePower(p abi.StoragePower) error {
	s.State.ThisEpochRawBytePower = p
	return nil
}

func (s *state5) GetState() interface{} {
	return &s.State
}

func (s *state5) claims() (adt.Map, error) {
	return adt5.AsMap(s.store, s.Claims, builtin5.DefaultHamtBitwidth)
}

func (s *state5) decodeClaim(val *cbg.Deferred) (Claim, error) {
	var ci power5.Claim
	if err := ci.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return Claim{}, err
	}
	return fromV5Claim(ci), nil
}

func fromV5Claim(v5 power5.Claim) Claim {
	return Claim{
		RawBytePower:    v5.RawBytePower,
		QualityAdjPower: v5.QualityAdjPower,
	}
}
