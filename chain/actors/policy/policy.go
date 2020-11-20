package policy

import (
	"sort"

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
	paych2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/paych"
	verifreg2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"
)

const (
	ChainFinality          = miner0.ChainFinality
	SealRandomnessLookback = ChainFinality
	PaychSettleDelay       = paych2.SettleDelay
)

// SetSupportedProofTypes sets supported proof types, across all actor versions.
// This should only be used for testing.
func SetSupportedProofTypes(types ...abi.RegisteredSealProof) {
	miner0.SupportedProofTypes = make(map[abi.RegisteredSealProof]struct{}, len(types))
	miner2.PreCommitSealProofTypesV0 = make(map[abi.RegisteredSealProof]struct{}, len(types))
	miner2.PreCommitSealProofTypesV7 = make(map[abi.RegisteredSealProof]struct{}, len(types)*2)
	miner2.PreCommitSealProofTypesV8 = make(map[abi.RegisteredSealProof]struct{}, len(types))

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
	}
}

// SetPreCommitChallengeDelay sets the pre-commit challenge delay across all
// actors versions. Use for testing.
func SetPreCommitChallengeDelay(delay abi.ChainEpoch) {
	// Set for all miner versions.
	miner0.PreCommitChallengeDelay = delay
	miner2.PreCommitChallengeDelay = delay
}

// TODO: this function shouldn't really exist. Instead, the API should expose the precommit delay.
func GetPreCommitChallengeDelay() abi.ChainEpoch {
	return miner0.PreCommitChallengeDelay
}

// SetConsensusMinerMinPower sets the minimum power of an individual miner must
// meet for leader election, across all actor versions. This should only be used
// for testing.
func SetConsensusMinerMinPower(p abi.StoragePower) {
	power0.ConsensusMinerMinPower = p
	for _, policy := range builtin2.SealProofPolicies {
		policy.ConsensusMinerMinPower = p
	}
}

// SetMinVerifiedDealSize sets the minimum size of a verified deal. This should
// only be used for testing.
func SetMinVerifiedDealSize(size abi.StoragePower) {
	verifreg0.MinVerifiedDealSize = size
	verifreg2.MinVerifiedDealSize = size
}

func GetMaxProveCommitDuration(ver actors.Version, t abi.RegisteredSealProof) abi.ChainEpoch {
	switch ver {
	case actors.Version0:
		return miner0.MaxSealDuration[t]
	case actors.Version2:
		return miner2.MaxProveCommitDuration[t]
	default:
		panic("unsupported actors version")
	}
}

func DealProviderCollateralBounds(
	size abi.PaddedPieceSize, verified bool,
	rawBytePower, qaPower, baselinePower abi.StoragePower,
	circulatingFil abi.TokenAmount, nwVer network.Version,
) (min, max abi.TokenAmount) {
	switch actors.VersionForNetwork(nwVer) {
	case actors.Version0:
		return market0.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil, nwVer)
	case actors.Version2:
		return market2.DealProviderCollateralBounds(size, verified, rawBytePower, qaPower, baselinePower, circulatingFil)
	default:
		panic("unsupported network version")
	}
}

// Sets the challenge window and scales the proving period to match (such that
// there are always 48 challenge windows in a proving period).
func SetWPoStChallengeWindow(period abi.ChainEpoch) {
	miner0.WPoStChallengeWindow = period
	miner0.WPoStProvingPeriod = period * abi.ChainEpoch(miner0.WPoStPeriodDeadlines)

	miner2.WPoStChallengeWindow = period
	miner2.WPoStProvingPeriod = period * abi.ChainEpoch(miner2.WPoStPeriodDeadlines)
}

func GetWinningPoStSectorSetLookback(nwVer network.Version) abi.ChainEpoch {
	if nwVer <= network.Version3 {
		return 10
	}

	return ChainFinality
}

func GetMaxSectorExpirationExtension() abi.ChainEpoch {
	return miner0.MaxSectorExpirationExtension
}

// TODO: we'll probably need to abstract over this better in the future.
func GetMaxPoStPartitions(p abi.RegisteredPoStProof) (int, error) {
	sectorsPerPart, err := builtin2.PoStProofWindowPoStPartitionSectors(p)
	if err != nil {
		return 0, err
	}
	return int(miner2.AddressedSectorsMax / sectorsPerPart), nil
}

func GetDefaultSectorSize() abi.SectorSize {
	// supported sector sizes are the same across versions.
	szs := make([]abi.SectorSize, 0, len(miner2.PreCommitSealProofTypesV8))
	for spt := range miner2.PreCommitSealProofTypesV8 {
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
