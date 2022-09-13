package power

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/actors"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"

	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	builtin8 "github.com/filecoin-project/go-state-types/builtin"
)

var (
	Address = builtin8.StoragePowerActorAddr
	Methods = builtin8.MethodsPower
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != actors.PowerKey {
			return nil, xerrors.Errorf("actor code is not power: %s", name)
		}

		switch av {

		case actors.Version8:
			return load8(store, act.Head)

		}
	}

	switch act.Code {

	case builtin0.StoragePowerActorCodeID:
		return load0(store, act.Head)

	case builtin2.StoragePowerActorCodeID:
		return load2(store, act.Head)

	case builtin3.StoragePowerActorCodeID:
		return load3(store, act.Head)

	case builtin4.StoragePowerActorCodeID:
		return load4(store, act.Head)

	case builtin5.StoragePowerActorCodeID:
		return load5(store, act.Head)

	case builtin6.StoragePowerActorCodeID:
		return load6(store, act.Head)

	case builtin7.StoragePowerActorCodeID:
		return load7(store, act.Head)

	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actors.Version) (State, error) {
	switch av {

	case actors.Version0:
		return make0(store)

	case actors.Version2:
		return make2(store)

	case actors.Version3:
		return make3(store)

	case actors.Version4:
		return make4(store)

	case actors.Version5:
		return make5(store)

	case actors.Version6:
		return make6(store)

	case actors.Version7:
		return make7(store)

	case actors.Version8:
		return make8(store)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	cbor.Marshaler

	TotalLocked() (abi.TokenAmount, error)
	TotalPower() (Claim, error)
	TotalCommitted() (Claim, error)
	TotalPowerSmoothed() (builtin.FilterEstimate, error)
	GetState() interface{}

	// MinerCounts returns the number of miners. Participating is the number
	// with power above the minimum miner threshold.
	MinerCounts() (participating, total uint64, err error)
	MinerPower(address.Address) (Claim, bool, error)
	MinerNominalPowerMeetsConsensusMinimum(address.Address) (bool, error)
	ListAllMiners() ([]address.Address, error)
	ForEachClaim(func(miner address.Address, claim Claim) error) error
	ClaimsChanged(State) (bool, error)

	// Testing or genesis setup only
	SetTotalQualityAdjPower(abi.StoragePower) error
	SetTotalRawBytePower(abi.StoragePower) error
	SetThisEpochQualityAdjPower(abi.StoragePower) error
	SetThisEpochRawBytePower(abi.StoragePower) error

	// Diff helpers. Used by Diff* functions internally.
	claims() (adt.Map, error)
	decodeClaim(*cbg.Deferred) (Claim, error)
}

type Claim struct {
	// Sum of raw byte power for a miner's sectors.
	RawBytePower abi.StoragePower

	// Sum of quality adjusted power for a miner's sectors.
	QualityAdjPower abi.StoragePower
}

func AddClaims(a Claim, b Claim) Claim {
	return Claim{
		RawBytePower:    big.Add(a.RawBytePower, b.RawBytePower),
		QualityAdjPower: big.Add(a.QualityAdjPower, b.QualityAdjPower),
	}
}
