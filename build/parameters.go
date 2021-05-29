package build

import (
	"fmt"
	"os"
	"strconv"

	rice "github.com/GeertJohan/go.rice"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/ipfs/go-cid"
)

func ParametersJSON() []byte {
	return rice.MustFindBox("proof-params").MustBytes("parameters.json")
}

var networkParams perNetworkParams

func NetworkParams() perNetworkParams {
	return networkParams
}

type perNetworkParams interface {
	DrandSchedule() map[abi.ChainEpoch]DrandEnum
	BootstrappersFile() string
	GenesisFile() string
	UpgradeBreezeHeight() abi.ChainEpoch
	BreezeGasTampingDuration() abi.ChainEpoch
	UpgradeSmokeHeight() abi.ChainEpoch
	UpgradeIgnitionHeight() abi.ChainEpoch
	UpgradeRefuelHeight() abi.ChainEpoch
	UpgradeActorsV2Height() abi.ChainEpoch
	UpgradeTapeHeight() abi.ChainEpoch
	UpgradeLiftoffHeight() abi.ChainEpoch
	UpgradeKumquatHeight() abi.ChainEpoch
	UpgradeCalicoHeight() abi.ChainEpoch
	UpgradePersianHeight() abi.ChainEpoch
	UpgradeOrangeHeight() abi.ChainEpoch
	UpgradeClausHeight() abi.ChainEpoch
	UpgradeActorsV3Height() abi.ChainEpoch
	UpgradeNorwegianHeight() abi.ChainEpoch
	UpgradeActorsV4Height() abi.ChainEpoch
	BlockDelaySecs() uint64
	PropagationDelaySecs() uint64
	BootstrapPeerThreshold() int
	WhitelistedBlock() cid.Cid
	InsecurePoStValidation() bool
	Devnet() bool
}

type sharedNetworkPrams interface {
}

type combinedNetworkParams interface {
	perNetworkParams
	sharedNetworkPrams
}

func init() {
	switch os.Getenv("LOTUS_NETWORK") {
	case "2k":
		fmt.Println("2k network params will be used")
		policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1, abi.RegisteredSealProof_StackedDrg8MiBV1)
		policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
		policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
		policy.SetPreCommitChallengeDelay(abi.ChainEpoch(10))

		getUpgradeHeight := func(ev string, def abi.ChainEpoch) abi.ChainEpoch {
			hs, found := os.LookupEnv(ev)
			if found {
				h, err := strconv.Atoi(hs)
				if err != nil {
					panic(fmt.Errorf("failed to parse %s env var", err))
				}

				return abi.ChainEpoch(h)
			}
			return def
		}

		twokUpgradeBreezeHeight = getUpgradeHeight("LOTUS_BREEZE_HEIGHT", twokUpgradeBreezeHeight)
		twokUpgradeSmokeHeight = getUpgradeHeight("LOTUS_SMOKE_HEIGHT", twokUpgradeSmokeHeight)
		twokUpgradeIgnitionHeight = getUpgradeHeight("LOTUS_IGNITION_HEIGHT", twokUpgradeIgnitionHeight)
		twokUpgradeRefuelHeight = getUpgradeHeight("LOTUS_REFUEL_HEIGHT", twokUpgradeRefuelHeight)
		twokUpgradeTapeHeight = getUpgradeHeight("LOTUS_TAPE_HEIGHT", twokUpgradeTapeHeight)
		twokUpgradeActorsV2Height = getUpgradeHeight("LOTUS_ACTORSV2_HEIGHT", twokUpgradeActorsV2Height)
		twokUpgradeLiftoffHeight = getUpgradeHeight("LOTUS_LIFTOFF_HEIGHT", twokUpgradeLiftoffHeight)
		twokUpgradeKumquatHeight = getUpgradeHeight("LOTUS_KUMQUAT_HEIGHT", twokUpgradeKumquatHeight)
		twokUpgradeCalicoHeight = getUpgradeHeight("LOTUS_CALICO_HEIGHT", twokUpgradeCalicoHeight)
		twokUpgradePersianHeight = getUpgradeHeight("LOTUS_PERSIAN_HEIGHT", twokUpgradePersianHeight)
		twokUpgradeOrangeHeight = getUpgradeHeight("LOTUS_ORANGE_HEIGHT", twokUpgradeOrangeHeight)
		twokUpgradeClausHeight = getUpgradeHeight("LOTUS_CLAUS_HEIGHT", twokUpgradeClausHeight)
		twokUpgradeActorsV3Height = getUpgradeHeight("LOTUS_ACTORSV3_HEIGHT", twokUpgradeActorsV3Height)
		twokUpgradeNorwegianHeight = getUpgradeHeight("LOTUS_NORWEGIAN_HEIGHT", twokUpgradeNorwegianHeight)
		twokUpgradeActorsV4Height = getUpgradeHeight("LOTUS_ACTORSV4_HEIGHT", twokUpgradeActorsV4Height)
		SetAddressNetwork(address.Testnet)
		networkParams = twokConfigurableParams{}
		return
	case "butterfly", "butterflynet":
		policy.SetConsensusMinerMinPower(abi.NewStoragePower(2 << 30))
		policy.SetSupportedProofTypes(
			abi.RegisteredSealProof_StackedDrg512MiBV1,
		)

		SetAddressNetwork(address.Testnet)
		networkParams = butterflyConfigurableParams{}
		return
	case "nerpa", "nerpanet":
		// Minimum block production power is set to 4 TiB
		// Rationale is to discourage small-scale miners from trying to take over the network
		// One needs to invest in ~2.3x the compute to break consensus, making it not worth it
		//
		// DOWNSIDE: the fake-seals need to be kept alive/protected, otherwise network will seize
		//
		policy.SetConsensusMinerMinPower(abi.NewStoragePower(4 << 40))

		policy.SetSupportedProofTypes(
			abi.RegisteredSealProof_StackedDrg512MiBV1,
			abi.RegisteredSealProof_StackedDrg32GiBV1,
			abi.RegisteredSealProof_StackedDrg64GiBV1,
		)

		// Lower the most time-consuming parts of PoRep
		policy.SetPreCommitChallengeDelay(10)

		// TODO - make this a variable
		//miner.WPoStChallengeLookback = abi.ChainEpoch(2)
		networkParams = nerpaConfigurableParams{}
		return
	case "calibration", "calibrationnet":
		policy.SetConsensusMinerMinPower(abi.NewStoragePower(32 << 30))
		policy.SetSupportedProofTypes(
			abi.RegisteredSealProof_StackedDrg32GiBV1,
			abi.RegisteredSealProof_StackedDrg64GiBV1,
		)
		SetAddressNetwork(address.Testnet)
		networkParams = calibrationConfigurableParams{}
		return
	}
	// by default, be mainnet.
	if os.Getenv("LOTUS_USE_TEST_ADDRESSES") != "1" {
		SetAddressNetwork(address.Mainnet)
	}
	networkParams = mainConfigurableParams{}
}
