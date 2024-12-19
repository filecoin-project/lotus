//go:build debug || 2k
// +build debug 2k

package buildconstants

import (
	"os"
	"strconv"
	"strings"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
)

const BootstrappersFile = ""
const GenesisFile = ""

var NetworkBundle = "devnet"
var ActorDebugging = true

var GenesisNetworkVersion = network.Version24

var UpgradeBreezeHeight = abi.ChainEpoch(-1)

const BreezeGasTampingDuration = 0

var UpgradeSmokeHeight = abi.ChainEpoch(-1)
var UpgradeIgnitionHeight = abi.ChainEpoch(-2)
var UpgradeRefuelHeight = abi.ChainEpoch(-3)
var UpgradeTapeHeight = abi.ChainEpoch(-4)

var UpgradeAssemblyHeight = abi.ChainEpoch(-5)
var UpgradeLiftoffHeight = abi.ChainEpoch(-6)

var UpgradeKumquatHeight = abi.ChainEpoch(-7)
var UpgradeCalicoHeight = abi.ChainEpoch(-9)
var UpgradePersianHeight = abi.ChainEpoch(-10)
var UpgradeOrangeHeight = abi.ChainEpoch(-11)
var UpgradeClausHeight = abi.ChainEpoch(-12)

var UpgradeTrustHeight = abi.ChainEpoch(-13)

var UpgradeNorwegianHeight = abi.ChainEpoch(-14)

var UpgradeTurboHeight = abi.ChainEpoch(-15)

var UpgradeHyperdriveHeight = abi.ChainEpoch(-16)

var UpgradeChocolateHeight = abi.ChainEpoch(-17)

var UpgradeOhSnapHeight = abi.ChainEpoch(-18)

var UpgradeSkyrHeight = abi.ChainEpoch(-19)

var UpgradeSharkHeight = abi.ChainEpoch(-20)

var UpgradeHyggeHeight = abi.ChainEpoch(-21)

var UpgradeLightningHeight = abi.ChainEpoch(-22)

var UpgradeThunderHeight = abi.ChainEpoch(-23)

var UpgradeWatermelonHeight = abi.ChainEpoch(-24)

var UpgradeDragonHeight = abi.ChainEpoch(-24)

var UpgradePhoenixHeight = abi.ChainEpoch(-25)

var UpgradeWaffleHeight = abi.ChainEpoch(-26)

var UpgradeTuktukHeight = abi.ChainEpoch(-27)

var UpgradeTeepHeight = abi.ChainEpoch(200)

// FIP-0081: for the power actor state for pledge calculations.
// UpgradeTuktukPowerRampDurationEpochs ends up in the power actor state after
// Tuktuk migration. along with a RampStartEpoch matching the upgrade height.
var UpgradeTuktukPowerRampDurationEpochs uint64 = 200

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFixHeight = -100

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFix2Height = -101

// This fix upgrade only ran on calibrationnet
const UpgradeCalibrationDragonFixHeight = -102

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandQuicknet,
}

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg2KiBV1,
	abi.RegisteredSealProof_StackedDrg8MiBV1,
}
var ConsensusMinerMinPower = abi.NewStoragePower(2048)
var PreCommitChallengeDelay = abi.ChainEpoch(10)

func init() {
	SetAddressNetwork(address.Testnet)
	getGenesisNetworkVersion := func(ev string, def network.Version) network.Version {
		hs, found := os.LookupEnv(ev)
		if found {
			h, err := strconv.Atoi(hs)
			if err != nil {
				log.Panicf("failed to parse %s env var", ev)
			}

			return network.Version(h)
		}

		return def
	}

	GenesisNetworkVersion = getGenesisNetworkVersion("LOTUS_GENESIS_NETWORK_VERSION", GenesisNetworkVersion)

	getBoolean := func(ev string, def bool) bool {
		hs, found := os.LookupEnv(ev)
		if found {
			return hs == "1" || strings.ToLower(hs) == "true"
		}

		return def
	}

	getUpgradeHeight := func(ev string, def abi.ChainEpoch) abi.ChainEpoch {
		hs, found := os.LookupEnv(ev)
		if found {
			h, err := strconv.Atoi(hs)
			if err != nil {
				log.Panicf("failed to parse %s env var", ev)
			}

			return abi.ChainEpoch(h)
		}

		return def
	}

	UpgradeBreezeHeight = getUpgradeHeight("LOTUS_BREEZE_HEIGHT", UpgradeBreezeHeight)
	UpgradeSmokeHeight = getUpgradeHeight("LOTUS_SMOKE_HEIGHT", UpgradeSmokeHeight)
	UpgradeIgnitionHeight = getUpgradeHeight("LOTUS_IGNITION_HEIGHT", UpgradeIgnitionHeight)
	UpgradeRefuelHeight = getUpgradeHeight("LOTUS_REFUEL_HEIGHT", UpgradeRefuelHeight)
	UpgradeTapeHeight = getUpgradeHeight("LOTUS_TAPE_HEIGHT", UpgradeTapeHeight)
	UpgradeAssemblyHeight = getUpgradeHeight("LOTUS_ACTORSV2_HEIGHT", UpgradeAssemblyHeight)
	UpgradeLiftoffHeight = getUpgradeHeight("LOTUS_LIFTOFF_HEIGHT", UpgradeLiftoffHeight)
	UpgradeKumquatHeight = getUpgradeHeight("LOTUS_KUMQUAT_HEIGHT", UpgradeKumquatHeight)
	UpgradeCalicoHeight = getUpgradeHeight("LOTUS_CALICO_HEIGHT", UpgradeCalicoHeight)
	UpgradePersianHeight = getUpgradeHeight("LOTUS_PERSIAN_HEIGHT", UpgradePersianHeight)
	UpgradeOrangeHeight = getUpgradeHeight("LOTUS_ORANGE_HEIGHT", UpgradeOrangeHeight)
	UpgradeClausHeight = getUpgradeHeight("LOTUS_CLAUS_HEIGHT", UpgradeClausHeight)
	UpgradeTrustHeight = getUpgradeHeight("LOTUS_ACTORSV3_HEIGHT", UpgradeTrustHeight)
	UpgradeNorwegianHeight = getUpgradeHeight("LOTUS_NORWEGIAN_HEIGHT", UpgradeNorwegianHeight)
	UpgradeTurboHeight = getUpgradeHeight("LOTUS_ACTORSV4_HEIGHT", UpgradeTurboHeight)
	UpgradeHyperdriveHeight = getUpgradeHeight("LOTUS_HYPERDRIVE_HEIGHT", UpgradeHyperdriveHeight)
	UpgradeChocolateHeight = getUpgradeHeight("LOTUS_CHOCOLATE_HEIGHT", UpgradeChocolateHeight)
	UpgradeOhSnapHeight = getUpgradeHeight("LOTUS_OHSNAP_HEIGHT", UpgradeOhSnapHeight)
	UpgradeSkyrHeight = getUpgradeHeight("LOTUS_SKYR_HEIGHT", UpgradeSkyrHeight)
	UpgradeSharkHeight = getUpgradeHeight("LOTUS_SHARK_HEIGHT", UpgradeSharkHeight)
	UpgradeHyggeHeight = getUpgradeHeight("LOTUS_HYGGE_HEIGHT", UpgradeHyggeHeight)
	UpgradeLightningHeight = getUpgradeHeight("LOTUS_LIGHTNING_HEIGHT", UpgradeLightningHeight)
	UpgradeThunderHeight = getUpgradeHeight("LOTUS_THUNDER_HEIGHT", UpgradeThunderHeight)
	UpgradeWatermelonHeight = getUpgradeHeight("LOTUS_WATERMELON_HEIGHT", UpgradeWatermelonHeight)
	UpgradeDragonHeight = getUpgradeHeight("LOTUS_DRAGON_HEIGHT", UpgradeDragonHeight)
	UpgradeWaffleHeight = getUpgradeHeight("LOTUS_WAFFLE_HEIGHT", UpgradeWaffleHeight)
	UpgradePhoenixHeight = getUpgradeHeight("LOTUS_PHOENIX_HEIGHT", UpgradePhoenixHeight)
	UpgradeTuktukHeight = getUpgradeHeight("LOTUS_TUKTUK_HEIGHT", UpgradeTuktukHeight)
	UpgradeTeepHeight = getUpgradeHeight("LOTUS_TEEP_HEIGHT", UpgradeTeepHeight)

	DrandSchedule = map[abi.ChainEpoch]DrandEnum{
		0: DrandQuicknet,
	}

	F3Enabled = getBoolean("LOTUS_F3_ENABLED", F3Enabled)
	F3BootstrapEpoch = getUpgradeHeight("LOTUS_F3_BOOTSTRAP_EPOCH", F3BootstrapEpoch)

	BuildType |= Build2k

}

const BlockDelaySecs = uint64(4)

const PropagationDelaySecs = uint64(1)

var EquivocationDelaySecs = uint64(0)

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const SlashablePowerDelay = 20

// Epochs
const InteractivePoRepConfidence = 6

const BootstrapPeerThreshold = 1

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = 31415926

var WhitelistedBlock = cid.Undef

var F3Enabled = true

var F3ManifestServerID = MustParseID("12D3KooWHcNBkqXEBrsjoveQvj6zDF3vK5S9tAfqyYaQF1LGSJwG")

// The initial F3 power table CID.
var F3InitialPowerTableCID cid.Cid = cid.Undef

var F3BootstrapEpoch abi.ChainEpoch = 1000
