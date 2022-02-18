package policy

import (
	"sort"

	"github.com/filecoin-project/go-state-types/big"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/actors"

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

	paych7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/paych"
)

const (
	ChainFinality                  = miner7.ChainFinality
	SealRandomnessLookback         = ChainFinality
	PaychSettleDelay               = paych7.SettleDelay
	MaxPreCommitRandomnessLookback = builtin7.EpochsInDay + SealRandomnessLookback
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

}

// TODO: this function shouldn't really exist. Instead, the API should expose the precommit delay.
func GetPreCommitChallengeDelay() abi.ChainEpoch {
	return miner7.PreCommitChallengeDelay
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

}

func GetMaxProveCommitDuration(ver actors.Version, t abi.RegisteredSealProof) (abi.ChainEpoch, error) {
	switch ver {

	case actors.Version0:

		return miner0.MaxSealDuration[t], nil

	case actors.Version2:

		return miner2.MaxProveCommitDuration[t], nil

	case actors.Version3:

		return miner3.MaxProveCommitDuration[t], nil

	case actors.Version4:

		return miner4.MaxProveCommitDuration[t], nil

	case actors.Version5:

		return miner5.MaxProveCommitDuration[t], nil

	case actors.Version6:

		return miner6.MaxProveCommitDuration[t], nil

	case actors.Version7:

		return miner7.MaxProveCommitDuration[t], nil

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

}

func DealProviderCollateralBounds(
	size abi.PaddedPieceSize, verified bool,
	rawBytePower, qaPower, baselinePower abi.StoragePower,
	circulatingFil abi.TokenAmount, nwVer network.Version,
) (min, max abi.TokenAmount, err error) {
	v, err := actors.VersionForNetwork(nwVer)
	if err != nil {
		return big.Zero(), big.Zero(), err
	}
	switch v {

	case actors.Version0:

		min, max := market0.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil, nwVer)
		return min, max, nil

	case actors.Version2:

		min, max := market2.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actors.Version3:

		min, max := market3.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actors.Version4:

		min, max := market4.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actors.Version5:

		min, max := market5.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actors.Version6:

		min, max := market6.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	case actors.Version7:

		min, max := market7.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
		return min, max, nil

	default:
		return big.Zero(), big.Zero(), xerrors.Errorf("unsupported actors version")
	}
}

func DealDurationBounds(pieceSize abi.PaddedPieceSize) (min, max abi.ChainEpoch) {
	return market7.DealDurationBounds(pieceSize)
}

// Sets the challenge window and scales the proving period to match (such that
// there are always 48 challenge windows in a proving period).
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

}

func GetWinningPoStSectorSetLookback(nwVer network.Version) abi.ChainEpoch {
	if nwVer <= network.Version3 {
		return 10
	}

	// NOTE: if this ever changes, adjust it in a (*Miner).mineOne() logline as well
	return ChainFinality
}

func GetMaxSectorExpirationExtension() abi.ChainEpoch {
	return miner7.MaxSectorExpirationExtension
}

func GetMinSectorExpiration() abi.ChainEpoch {
	return miner7.MinSectorExpiration
}

func GetMaxPoStPartitions(nv network.Version, p abi.RegisteredPoStProof) (int, error) {
	sectorsPerPart, err := builtin7.PoStProofWindowPoStPartitionSectors(p)
	if err != nil {
		return 0, err
	}
	maxSectors, err := GetAddressedSectorsMax(nv)
	if err != nil {
		return 0, err
	}
	return int(uint64(maxSectors) / sectorsPerPart), nil
}

func GetDefaultSectorSize() abi.SectorSize {
	// supported sector sizes are the same across versions.
	szs := make([]abi.SectorSize, 0, len(miner7.PreCommitSealProofTypesV8))
	for spt := range miner7.PreCommitSealProofTypesV8 {
		ss, err := spt.SectorSize()
		if err != nil {
			panic(err)
		}

		szs = append(szs, ss)
	}

	sort.Slice(szs, func(i, j int) bool {
		return szs[i] < szs[j]
	})

	return szs[0]
}

func GetDefaultAggregationProof() abi.RegisteredAggregationProof {
	return abi.RegisteredAggregationProof_SnarkPackV1
}

func GetSectorMaxLifetime(proof abi.RegisteredSealProof, nwVer network.Version) abi.ChainEpoch {
	if nwVer <= network.Version10 {
		return builtin4.SealProofPoliciesV0[proof].SectorMaxLifetime
	}

	return builtin7.SealProofPoliciesV11[proof].SectorMaxLifetime
}

func GetAddressedSectorsMax(nwVer network.Version) (int, error) {
	v, err := actors.VersionForNetwork(nwVer)
	if err != nil {
		return 0, err
	}
	switch v {

	case actors.Version0:
		return miner0.AddressedSectorsMax, nil

	case actors.Version2:
		return miner2.AddressedSectorsMax, nil

	case actors.Version3:
		return miner3.AddressedSectorsMax, nil

	case actors.Version4:
		return miner4.AddressedSectorsMax, nil

	case actors.Version5:
		return miner5.AddressedSectorsMax, nil

	case actors.Version6:
		return miner6.AddressedSectorsMax, nil

	case actors.Version7:
		return miner7.AddressedSectorsMax, nil

	default:
		return 0, xerrors.Errorf("unsupported network version")
	}
}

func GetDeclarationsMax(nwVer network.Version) (int, error) {
	v, err := actors.VersionForNetwork(nwVer)
	if err != nil {
		return 0, err
	}
	switch v {

	case actors.Version0:

		// TODO: Should we instead error here since the concept doesn't exist yet?
		return miner0.AddressedPartitionsMax, nil

	case actors.Version2:

		return miner2.DeclarationsMax, nil

	case actors.Version3:

		return miner3.DeclarationsMax, nil

	case actors.Version4:

		return miner4.DeclarationsMax, nil

	case actors.Version5:

		return miner5.DeclarationsMax, nil

	case actors.Version6:

		return miner6.DeclarationsMax, nil

	case actors.Version7:

		return miner7.DeclarationsMax, nil

	default:
		return 0, xerrors.Errorf("unsupported network version")
	}
}

func AggregateProveCommitNetworkFee(nwVer network.Version, aggregateSize int, baseFee abi.TokenAmount) (abi.TokenAmount, error) {
	v, err := actors.VersionForNetwork(nwVer)
	if err != nil {
		return big.Zero(), err
	}
	switch v {

	case actors.Version0:

		return big.Zero(), nil

	case actors.Version2:

		return big.Zero(), nil

	case actors.Version3:

		return big.Zero(), nil

	case actors.Version4:

		return big.Zero(), nil

	case actors.Version5:

		return miner5.AggregateNetworkFee(aggregateSize, baseFee), nil

	case actors.Version6:

		return miner6.AggregateProveCommitNetworkFee(aggregateSize, baseFee), nil

	case actors.Version7:

		return miner7.AggregateProveCommitNetworkFee(aggregateSize, baseFee), nil

	default:
		return big.Zero(), xerrors.Errorf("unsupported network version")
	}
}

func AggregatePreCommitNetworkFee(nwVer network.Version, aggregateSize int, baseFee abi.TokenAmount) (abi.TokenAmount, error) {
	v, err := actors.VersionForNetwork(nwVer)
	if err != nil {
		return big.Zero(), err
	}
	switch v {

	case actors.Version0:

		return big.Zero(), nil

	case actors.Version2:

		return big.Zero(), nil

	case actors.Version3:

		return big.Zero(), nil

	case actors.Version4:

		return big.Zero(), nil

	case actors.Version5:

		return big.Zero(), nil

	case actors.Version6:

		return miner6.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil

	case actors.Version7:

		return miner7.AggregatePreCommitNetworkFee(aggregateSize, baseFee), nil

	default:
		return big.Zero(), xerrors.Errorf("unsupported network version")
	}
}
