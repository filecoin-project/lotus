package policy

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	builtin10 "github.com/filecoin-project/go-state-types/builtin"
	builtin11 "github.com/filecoin-project/go-state-types/builtin"
	builtin12 "github.com/filecoin-project/go-state-types/builtin"
	builtin13 "github.com/filecoin-project/go-state-types/builtin"
	builtin14 "github.com/filecoin-project/go-state-types/builtin"
	builtin15 "github.com/filecoin-project/go-state-types/builtin"
	builtin16 "github.com/filecoin-project/go-state-types/builtin"
	builtin17 "github.com/filecoin-project/go-state-types/builtin"
	builtin18 "github.com/filecoin-project/go-state-types/builtin"
	builtin8 "github.com/filecoin-project/go-state-types/builtin"
	builtin9 "github.com/filecoin-project/go-state-types/builtin"
	market10 "github.com/filecoin-project/go-state-types/builtin/v10/market"
	miner10 "github.com/filecoin-project/go-state-types/builtin/v10/miner"
	verifreg10 "github.com/filecoin-project/go-state-types/builtin/v10/verifreg"
	market11 "github.com/filecoin-project/go-state-types/builtin/v11/market"
	miner11 "github.com/filecoin-project/go-state-types/builtin/v11/miner"
	verifreg11 "github.com/filecoin-project/go-state-types/builtin/v11/verifreg"
	market12 "github.com/filecoin-project/go-state-types/builtin/v12/market"
	miner12 "github.com/filecoin-project/go-state-types/builtin/v12/miner"
	verifreg12 "github.com/filecoin-project/go-state-types/builtin/v12/verifreg"
	market13 "github.com/filecoin-project/go-state-types/builtin/v13/market"
	miner13 "github.com/filecoin-project/go-state-types/builtin/v13/miner"
	verifreg13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	market14 "github.com/filecoin-project/go-state-types/builtin/v14/market"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	verifreg14 "github.com/filecoin-project/go-state-types/builtin/v14/verifreg"
	market15 "github.com/filecoin-project/go-state-types/builtin/v15/market"
	miner15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	verifreg15 "github.com/filecoin-project/go-state-types/builtin/v15/verifreg"
	market16 "github.com/filecoin-project/go-state-types/builtin/v16/market"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	verifreg16 "github.com/filecoin-project/go-state-types/builtin/v16/verifreg"
	market17 "github.com/filecoin-project/go-state-types/builtin/v17/market"
	miner17 "github.com/filecoin-project/go-state-types/builtin/v17/miner"
	verifreg17 "github.com/filecoin-project/go-state-types/builtin/v17/verifreg"
	market18 "github.com/filecoin-project/go-state-types/builtin/v18/market"
	miner18 "github.com/filecoin-project/go-state-types/builtin/v18/miner"
	paych18 "github.com/filecoin-project/go-state-types/builtin/v18/paych"
	verifreg18 "github.com/filecoin-project/go-state-types/builtin/v18/verifreg"
	market8 "github.com/filecoin-project/go-state-types/builtin/v8/market"
	miner8 "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	verifreg8 "github.com/filecoin-project/go-state-types/builtin/v8/verifreg"
	market9 "github.com/filecoin-project/go-state-types/builtin/v9/market"
	miner9 "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	verifreg9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/network"
	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	power0 "github.com/filecoin-project/specs-actors/actors/builtin/power"
	verifreg0 "github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	verifreg2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	market3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/market"
	miner3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/miner"
	verifreg3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/verifreg"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	market4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/market"
	miner4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/miner"
	verifreg4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/verifreg"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	market5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/market"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	verifreg5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/verifreg"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	market6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/market"
	miner6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/miner"
	verifreg6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/verifreg"
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"
	market7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/market"
	miner7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/miner"
	verifreg7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/verifreg"
)

const (
	ChainFinality                  = miner18.ChainFinality
	SealRandomnessLookback         = ChainFinality
	PaychSettleDelay               = paych18.SettleDelay
	MaxPreCommitRandomnessLookback = builtin18.EpochsInDay + SealRandomnessLookback
	DeclarationsMax                = 3000
)

var (
	MarketDefaultAllocationTermBuffer = market18.MarketDefaultAllocationTermBuffer
)

// SetSupportedProofTypes sets supported proof types, across all actor versions.
// This should only be used for testing.
func SetSupportedProofTypes(types ...abi.RegisteredSealProof) {

	miner0.SupportedProofTypes = make(map[abi.RegisteredSealProof]struct{}, len(types))

	miner2.PreCommitSealProofTypesV0 = make(map[abi.RegisteredSealProof]struct{}, len(types))
	miner2.PreCommitSealProofTypesV7 = make(map[abi.RegisteredSealProof]struct{}, len(types)*2)
	miner2.PreCommitSealProofTypesV8 = make(map[abi.RegisteredSealProof]struct{}, len(types))

	miner3.PreCommitSealProofTypesV0 = make(map[abi.RegisteredSealProof]struct{}, len(types))
	miner3.PreCommitSealProofTypesV7 = make(map[abi.RegisteredSealProof]struct{}, len(types)*2)
	miner3.PreCommitSealProofTypesV8 = make(map[abi.RegisteredSealProof]struct{}, len(types))

	miner4.PreCommitSealProofTypesV0 = make(map[abi.RegisteredSealProof]struct{}, len(types))
	miner4.PreCommitSealProofTypesV7 = make(map[abi.RegisteredSealProof]struct{}, len(types)*2)
	miner4.PreCommitSealProofTypesV8 = make(map[abi.RegisteredSealProof]struct{}, len(types))

	miner5.PreCommitSealProofTypesV8 = make(map[abi.RegisteredSealProof]struct{}, len(types))

	miner6.PreCommitSealProofTypesV8 = make(map[abi.RegisteredSealProof]struct{}, len(types))

	miner7.PreCommitSealProofTypesV8 = make(map[abi.RegisteredSealProof]struct{}, len(types))

	AddSupportedProofTypes(types...)
}

// AddSupportedProofTypes sets supported proof types, across all actor versions.
// This should only be used for testing.
func AddSupportedProofTypes(types ...abi.RegisteredSealProof) {
	for _, t := range types {
		if t >= abi.RegisteredSealProof_StackedDrg2KiBV1_1 {
			panic("must specify v1 proof types only")
		}
		// Set for all miner versions.

		miner0.SupportedProofTypes[t] = struct{}{}

		miner2.PreCommitSealProofTypesV0[t] = struct{}{}
		miner2.PreCommitSealProofTypesV7[t] = struct{}{}
		miner2.PreCommitSealProofTypesV7[t+abi.RegisteredSealProof_StackedDrg2KiBV1_1] = struct{}{}
		miner2.PreCommitSealProofTypesV8[t+abi.RegisteredSealProof_StackedDrg2KiBV1_1] = struct{}{}

		miner3.PreCommitSealProofTypesV0[t] = struct{}{}
		miner3.PreCommitSealProofTypesV7[t] = struct{}{}
		miner3.PreCommitSealProofTypesV7[t+abi.RegisteredSealProof_StackedDrg2KiBV1_1] = struct{}{}
		miner3.PreCommitSealProofTypesV8[t+abi.RegisteredSealProof_StackedDrg2KiBV1_1] = struct{}{}

		miner4.PreCommitSealProofTypesV0[t] = struct{}{}
		miner4.PreCommitSealProofTypesV7[t] = struct{}{}
		miner4.PreCommitSealProofTypesV7[t+abi.RegisteredSealProof_StackedDrg2KiBV1_1] = struct{}{}
		miner4.PreCommitSealProofTypesV8[t+abi.RegisteredSealProof_StackedDrg2KiBV1_1] = struct{}{}

		miner5.PreCommitSealProofTypesV8[t+abi.RegisteredSealProof_StackedDrg2KiBV1_1] = struct{}{}
		wpp, err := t.RegisteredWindowPoStProof()
		if err != nil {
			// Fine to panic, this is a test-only method
			panic(err)
		}

		miner5.WindowPoStProofTypes[wpp] = struct{}{}

		miner6.PreCommitSealProofTypesV8[t+abi.RegisteredSealProof_StackedDrg2KiBV1_1] = struct{}{}
		wpp, err = t.RegisteredWindowPoStProof()
		if err != nil {
			// Fine to panic, this is a test-only method
			panic(err)
		}

		miner6.WindowPoStProofTypes[wpp] = struct{}{}

		miner7.PreCommitSealProofTypesV8[t+abi.RegisteredSealProof_StackedDrg2KiBV1_1] = struct{}{}
		wpp, err = t.RegisteredWindowPoStProof()
		if err != nil {
			// Fine to panic, this is a test-only method
			panic(err)
		}

		miner7.WindowPoStProofTypes[wpp] = struct{}{}

	}
}

// SetPreCommitChallengeDelay sets the pre-commit challenge delay across all
// actors versions. Use for testing.
func SetPreCommitChallengeDelay(delay abi.ChainEpoch) {
	// Set for all miner versions.

	miner0.PreCommitChallengeDelay = delay

	miner2.PreCommitChallengeDelay = delay

	miner3.PreCommitChallengeDelay = delay

	miner4.PreCommitChallengeDelay = delay

	miner5.PreCommitChallengeDelay = delay

	miner6.PreCommitChallengeDelay = delay

	miner7.PreCommitChallengeDelay = delay

	miner8.PreCommitChallengeDelay = delay

	miner9.PreCommitChallengeDelay = delay

	miner10.PreCommitChallengeDelay = delay

	miner11.PreCommitChallengeDelay = delay

	miner12.PreCommitChallengeDelay = delay

	miner13.PreCommitChallengeDelay = delay

	miner14.PreCommitChallengeDelay = delay

	miner15.PreCommitChallengeDelay = delay

	miner16.PreCommitChallengeDelay = delay

	miner17.PreCommitChallengeDelay = delay

	miner18.PreCommitChallengeDelay = delay

}

func GetPreCommitChallengeDelay() abi.ChainEpoch {
	// TODO: this function shouldn't really exist. Instead, the API should expose the precommit delay.
	return miner18.PreCommitChallengeDelay
}

// SetConsensusMinerMinPower sets the minimum power of an individual miner must
// meet for leader election, across all actor versions. This should only be used
// for testing.
func SetConsensusMinerMinPower(p abi.StoragePower) {

	power0.ConsensusMinerMinPower = p

	for _, policy := range builtin2.SealProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin3.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin4.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin5.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin6.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin7.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin8.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin9.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin10.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin11.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin12.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin13.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin14.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin15.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin16.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin17.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

	for _, policy := range builtin18.PoStProofPolicies {
		policy.ConsensusMinerMinPower = p
	}

}

// SetMinVerifiedDealSize sets the minimum size of a verified deal. This should
// only be used for testing.
func SetMinVerifiedDealSize(size abi.StoragePower) {

	verifreg0.MinVerifiedDealSize = size

	verifreg2.MinVerifiedDealSize = size

	verifreg3.MinVerifiedDealSize = size

	verifreg4.MinVerifiedDealSize = size

	verifreg5.MinVerifiedDealSize = size

	verifreg6.MinVerifiedDealSize = size

	verifreg7.MinVerifiedDealSize = size

	verifreg8.MinVerifiedDealSize = size

	verifreg9.MinVerifiedDealSize = size

	verifreg10.MinVerifiedDealSize = size

	verifreg11.MinVerifiedDealSize = size

	verifreg12.MinVerifiedDealSize = size

	verifreg13.MinVerifiedDealSize = size

	verifreg14.MinVerifiedDealSize = size

	verifreg15.MinVerifiedDealSize = size

	verifreg16.MinVerifiedDealSize = size

	verifreg17.MinVerifiedDealSize = size

	verifreg18.MinVerifiedDealSize = size

}

func GetMaxProveCommitDuration(ver actorstypes.Version, t abi.RegisteredSealProof) (abi.ChainEpoch, error) {
	switch ver {

	case actorstypes.Version0:

		return miner0.MaxSealDuration[t], nil

	case actorstypes.Version2:

		return miner2.MaxProveCommitDuration[t], nil

	case actorstypes.Version3:

		return miner3.MaxProveCommitDuration[t], nil

	case actorstypes.Version4:

		return miner4.MaxProveCommitDuration[t], nil

	case actorstypes.Version5:

		return miner5.MaxProveCommitDuration[t], nil

	case actorstypes.Version6:

		return miner6.MaxProveCommitDuration[t], nil

	case actorstypes.Version7:

		return miner7.MaxProveCommitDuration[t], nil

	case actorstypes.Version8:

		return miner8.MaxProveCommitDuration[t], nil

	case actorstypes.Version9:

		return miner9.MaxProveCommitDuration[t], nil

	case actorstypes.Version10:

		return miner10.MaxProveCommitDuration[t], nil

	case actorstypes.Version11:

		return miner11.MaxProveCommitDuration[t], nil

	case actorstypes.Version12:

		return miner12.MaxProveCommitDuration[t], nil

	case actorstypes.Version13:

		return miner13.MaxProveCommitDuration[t], nil

	case actorstypes.Version14:

		return miner14.MaxProveCommitDuration[t], nil

	case actorstypes.Version15:

		return miner15.MaxProveCommitDuration[t], nil

	case actorstypes.Version16:

		return miner16.MaxProveCommitDuration[t], nil

	case actorstypes.Version17:

		return miner17.MaxProveCommitDuration[t], nil

	case actorstypes.Version18:

		return miner18.MaxProveCommitDuration[t], nil

	default:
		return 0, xerrors.Errorf("unsupported actors version")
	}
}

// SetProviderCollateralSupplyTarget sets the percentage of normalized circulating
// supply that must be covered by provider collateral in a deal. This should
// only be used for testing.
func SetProviderCollateralSupplyTarget(num, denom big.Int) {

	market2.ProviderCollateralSupplyTarget = builtin2.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market3.ProviderCollateralSupplyTarget = builtin3.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market4.ProviderCollateralSupplyTarget = builtin4.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market5.ProviderCollateralSupplyTarget = builtin5.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market6.ProviderCollateralSupplyTarget = builtin6.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market7.ProviderCollateralSupplyTarget = builtin7.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market8.ProviderCollateralSupplyTarget = builtin8.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market9.ProviderCollateralSupplyTarget = builtin9.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market10.ProviderCollateralSupplyTarget = builtin10.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market11.ProviderCollateralSupplyTarget = builtin11.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market12.ProviderCollateralSupplyTarget = builtin12.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market13.ProviderCollateralSupplyTarget = builtin13.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market14.ProviderCollateralSupplyTarget = builtin14.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market15.ProviderCollateralSupplyTarget = builtin15.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market16.ProviderCollateralSupplyTarget = builtin16.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market17.ProviderCollateralSupplyTarget = builtin17.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

	market18.ProviderCollateralSupplyTarget = builtin18.BigFrac{
		Numerator:   num,
		Denominator: denom,
	}

}

func DealProviderCollateralBounds(
	size abi.PaddedPieceSize, verified bool,
	rawBytePower, qaPower, baselinePower abi.StoragePower,
	circulatingFil abi.TokenAmount, nwVer network.Version,
) (min, max abi.TokenAmount, err error) {
	v, err := actorstypes.VersionForNetwork(nwVer)
	if err != nil {
		return big.Zero(), big.Zero(), err
	}
	switch v {

	case actorstypes.Version0:

		min, max := market0.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil, nwVer)
		return min, max, nil

	case actorstypes.Version2:

		min, max := market2.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version3:

		min, max := market3.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version4:

		min, max := market4.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version5:

		min, max := market5.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version6:

		min, max := market6.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version7:

		min, max := market7.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version8:

		min, max := market8.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version9:

		min, max := market9.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version10:

		min, max := market10.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version11:

		min, max := market11.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version12:

		min, max := market12.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version13:

		min, max := market13.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version14:

		min, max := market14.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version15:

		min, max := market15.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version16:

		min, max := market16.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version17:

		min, max := market17.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actorstypes.Version18:

		min, max := market18.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	default:
		return big.Zero(), big.Zero(), xerrors.Errorf("unsupported actors version")
	}
}

func DealDurationBounds(pieceSize abi.PaddedPieceSize) (min, max abi.ChainEpoch) {
	return market18.DealDurationBounds(pieceSize)
}

// SetWPoStChallengeWindow sets the challenge window and scales the proving period to match (such
// that there are always 48 challenge windows in a proving period).
func SetWPoStChallengeWindow(period abi.ChainEpoch) {

	miner0.WPoStChallengeWindow = period
	miner0.WPoStProvingPeriod = period * abi.ChainEpoch(miner0.WPoStPeriodDeadlines)

	miner2.WPoStChallengeWindow = period
	miner2.WPoStProvingPeriod = period * abi.ChainEpoch(miner2.WPoStPeriodDeadlines)

	miner3.WPoStChallengeWindow = period
	miner3.WPoStProvingPeriod = period * abi.ChainEpoch(miner3.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner3.WPoStDisputeWindow = period * 30

	miner4.WPoStChallengeWindow = period
	miner4.WPoStProvingPeriod = period * abi.ChainEpoch(miner4.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner4.WPoStDisputeWindow = period * 30

	miner5.WPoStChallengeWindow = period
	miner5.WPoStProvingPeriod = period * abi.ChainEpoch(miner5.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner5.WPoStDisputeWindow = period * 30

	miner6.WPoStChallengeWindow = period
	miner6.WPoStProvingPeriod = period * abi.ChainEpoch(miner6.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner6.WPoStDisputeWindow = period * 30

	miner7.WPoStChallengeWindow = period
	miner7.WPoStProvingPeriod = period * abi.ChainEpoch(miner7.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner7.WPoStDisputeWindow = period * 30

	miner8.WPoStChallengeWindow = period
	miner8.WPoStProvingPeriod = period * abi.ChainEpoch(miner8.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner8.WPoStDisputeWindow = period * 30

	miner9.WPoStChallengeWindow = period
	miner9.WPoStProvingPeriod = period * abi.ChainEpoch(miner9.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner9.WPoStDisputeWindow = period * 30

	miner10.WPoStChallengeWindow = period
	miner10.WPoStProvingPeriod = period * abi.ChainEpoch(miner10.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner10.WPoStDisputeWindow = period * 30

	miner11.WPoStChallengeWindow = period
	miner11.WPoStProvingPeriod = period * abi.ChainEpoch(miner11.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner11.WPoStDisputeWindow = period * 30

	miner12.WPoStChallengeWindow = period
	miner12.WPoStProvingPeriod = period * abi.ChainEpoch(miner12.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner12.WPoStDisputeWindow = period * 30

	miner13.WPoStChallengeWindow = period
	miner13.WPoStProvingPeriod = period * abi.ChainEpoch(miner13.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner13.WPoStDisputeWindow = period * 30

	miner14.WPoStChallengeWindow = period
	miner14.WPoStProvingPeriod = period * abi.ChainEpoch(miner14.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner14.WPoStDisputeWindow = period * 30

	miner15.WPoStChallengeWindow = period
	miner15.WPoStProvingPeriod = period * abi.ChainEpoch(miner15.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner15.WPoStDisputeWindow = period * 30

	miner16.WPoStChallengeWindow = period
	miner16.WPoStProvingPeriod = period * abi.ChainEpoch(miner16.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner16.WPoStDisputeWindow = period * 30

	miner17.WPoStChallengeWindow = period
	miner17.WPoStProvingPeriod = period * abi.ChainEpoch(miner17.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner17.WPoStDisputeWindow = period * 30

	miner18.WPoStChallengeWindow = period
	miner18.WPoStProvingPeriod = period * abi.ChainEpoch(miner18.WPoStPeriodDeadlines)

	// by default, this is 2x finality which is 30 periods.
	// scale it if we're scaling the challenge period.
	miner18.WPoStDisputeWindow = period * 30

}

func GetWinningPoStSectorSetLookback(nwVer network.Version) abi.ChainEpoch {
	if nwVer <= network.Version3 {
		return 10
	}

	// NOTE: if this ever changes, adjust it in a (*Miner).mineOne() logline as well
	return ChainFinality
}

func GetMaxSectorExpirationExtension(nv network.Version) (abi.ChainEpoch, error) {
	v, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return 0, xerrors.Errorf("failed to get actors version: %w", err)
	}
	switch v {

	case actorstypes.Version0:
		return miner0.MaxSectorExpirationExtension, nil

	case actorstypes.Version2:
		return miner2.MaxSectorExpirationExtension, nil

	case actorstypes.Version3:
		return miner3.MaxSectorExpirationExtension, nil

	case actorstypes.Version4:
		return miner4.MaxSectorExpirationExtension, nil

	case actorstypes.Version5:
		return miner5.MaxSectorExpirationExtension, nil

	case actorstypes.Version6:
		return miner6.MaxSectorExpirationExtension, nil

	case actorstypes.Version7:
		return miner7.MaxSectorExpirationExtension, nil

	case actorstypes.Version8:
		return miner8.MaxSectorExpirationExtension, nil

	case actorstypes.Version9:
		return miner9.MaxSectorExpirationExtension, nil

	case actorstypes.Version10:
		return miner10.MaxSectorExpirationExtension, nil

	case actorstypes.Version11:
		return miner11.MaxSectorExpirationExtension, nil

	case actorstypes.Version12:
		return miner12.MaxSectorExpirationExtension, nil

	case actorstypes.Version13:
		return miner13.MaxSectorExpirationExtension, nil

	case actorstypes.Version14:
		return miner14.MaxSectorExpirationExtension, nil

	case actorstypes.Version15:
		return miner15.MaxSectorExpirationExtension, nil

	case actorstypes.Version16:
		return miner16.MaxSectorExpirationExtension, nil

	case actorstypes.Version17:
		return miner17.MaxSectorExpirationExtension, nil

	case actorstypes.Version18:
		return miner18.MaxSectorExpirationExtension, nil

	default:
		return 0, xerrors.Errorf("unsupported network version")
	}

}

func GetMinSectorExpiration() abi.ChainEpoch {
	return miner18.MinSectorExpiration
}

func GetMaxPoStPartitions(nv network.Version, p abi.RegisteredPoStProof) (int, error) {
	sectorsPerPart, err := builtin18.PoStProofWindowPoStPartitionSectors(p)
	if err != nil {
		return 0, err
	}
	maxSectors, err := GetAddressedSectorsMax(nv)
	if err != nil {
		return 0, err
	}

	return min(miner18.PoStedPartitionsMax, int(uint64(maxSectors)/sectorsPerPart)), nil
}

func GetDefaultAggregationProof() abi.RegisteredAggregationProof {
	return abi.RegisteredAggregationProof_SnarkPackV1
}

func GetSectorMaxLifetime(proof abi.RegisteredSealProof, nwVer network.Version) abi.ChainEpoch {
	if nwVer <= network.Version10 {
		return builtin4.SealProofPoliciesV0[proof].SectorMaxLifetime
	}

	return builtin18.SealProofPoliciesV11[proof].SectorMaxLifetime
}

func GetAddressedSectorsMax(nwVer network.Version) (int, error) {
	v, err := actorstypes.VersionForNetwork(nwVer)
	if err != nil {
		return 0, err
	}
	switch v {

	case actorstypes.Version0:
		return miner0.AddressedSectorsMax, nil

	case actorstypes.Version2:
		return miner2.AddressedSectorsMax, nil

	case actorstypes.Version3:
		return miner3.AddressedSectorsMax, nil

	case actorstypes.Version4:
		return miner4.AddressedSectorsMax, nil

	case actorstypes.Version5:
		return miner5.AddressedSectorsMax, nil

	case actorstypes.Version6:
		return miner6.AddressedSectorsMax, nil

	case actorstypes.Version7:
		return miner7.AddressedSectorsMax, nil

	case actorstypes.Version8:
		return miner8.AddressedSectorsMax, nil

	case actorstypes.Version9:
		return miner9.AddressedSectorsMax, nil

	case actorstypes.Version10:
		return miner10.AddressedSectorsMax, nil

	case actorstypes.Version11:
		return miner11.AddressedSectorsMax, nil

	case actorstypes.Version12:
		return miner12.AddressedSectorsMax, nil

	case actorstypes.Version13:
		return miner13.AddressedSectorsMax, nil

	case actorstypes.Version14:
		return miner14.AddressedSectorsMax, nil

	case actorstypes.Version15:
		return miner15.AddressedSectorsMax, nil

	case actorstypes.Version16:
		return miner16.AddressedSectorsMax, nil

	case actorstypes.Version17:
		return miner17.AddressedSectorsMax, nil

	case actorstypes.Version18:
		return miner18.AddressedSectorsMax, nil

	default:
		return 0, xerrors.Errorf("unsupported network version")
	}
}

func AggregatePreCommitNetworkFee(nwVer network.Version, aggregateSize int, baseFee abi.TokenAmount) (abi.TokenAmount, error) {
	v, err := actorstypes.VersionForNetwork(nwVer)
	if err != nil {
		return big.Zero(), err
	}
	switch v {
	case actorstypes.Version0:
		return big.Zero(), nil
	case actorstypes.Version2:
		return big.Zero(), nil
	case actorstypes.Version3:
		return big.Zero(), nil
	case actorstypes.Version4:
		return big.Zero(), nil
	case actorstypes.Version5:
		return big.Zero(), nil
	case actorstypes.Version6:
		return miner6.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil
	case actorstypes.Version7:
		return miner7.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil
	case actorstypes.Version8:
		return miner8.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil
	case actorstypes.Version9:
		return miner9.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil
	case actorstypes.Version10:
		return miner10.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil
	case actorstypes.Version11:
		return miner11.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil
	case actorstypes.Version12:
		return miner12.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil
	case actorstypes.Version13:
		return miner13.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil
	case actorstypes.Version14:
		return miner14.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil
	case actorstypes.Version15:
		return miner15.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil
	case actorstypes.Version16:
		return big.Zero(), nil
	case actorstypes.Version17:
		return big.Zero(), nil
	case actorstypes.Version18:
		return big.Zero(), nil
	default:
		return big.Zero(), xerrors.Errorf("unsupported network version")
	}
}

var PoStToSealMap map[abi.RegisteredPoStProof]abi.RegisteredSealProof

func init() {
	PoStToSealMap = make(map[abi.RegisteredPoStProof]abi.RegisteredSealProof)
	for sealProof, info := range abi.SealProofInfos {
		PoStToSealMap[info.WinningPoStProof] = sealProof
		PoStToSealMap[info.WindowPoStProof] = sealProof
	}
}

func GetSealProofFromPoStProof(postProof abi.RegisteredPoStProof) (abi.RegisteredSealProof, error) {
	sealProof, exists := PoStToSealMap[postProof]
	if !exists {
		return 0, xerrors.New("no corresponding RegisteredSealProof for the given RegisteredPoStProof")
	}
	return sealProof, nil
}
