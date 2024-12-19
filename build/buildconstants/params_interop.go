//go:build interopnet
// +build interopnet

package buildconstants

import (
	"os"
	"strconv"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var NetworkBundle = "caterpillarnet"
var ActorDebugging = false

const BootstrappersFile = "interopnet.pi"
const GenesisFile = "interopnet.car.zst"

const GenesisNetworkVersion = network.Version22

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
var UpgradeDragonHeight = abi.ChainEpoch(-25)
var UpgradePhoenixHeight = abi.ChainEpoch(-26)
var UpgradeWaffleHeight = abi.ChainEpoch(-27)
var UpgradeTuktukHeight = abi.ChainEpoch(-28)

const UpgradeTeepHeight = 50

// FIP-0081: for the power actor state for pledge calculations.
// UpgradeTuktukPowerRampDurationEpochs ends up in the power actor state after
// Tuktuk migration. along with a RampStartEpoch matching the upgrade height.
var UpgradeTuktukPowerRampDurationEpochs = uint64(builtin.EpochsInYear)

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFixHeight = -1

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFix2Height = -2

// This fix upgrade only ran on calibrationnet
const UpgradeCalibrationDragonFixHeight = -3

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandQuicknet,
}

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg2KiBV1,
	abi.RegisteredSealProof_StackedDrg8MiBV1,
	abi.RegisteredSealProof_StackedDrg512MiBV1,
}
var ConsensusMinerMinPower = abi.NewStoragePower(2048)
var PreCommitChallengeDelay = abi.ChainEpoch(10)

func init() {
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

	BuildType |= BuildInteropnet
	SetAddressNetwork(address.Testnet)
	Devnet = true

}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)

var EquivocationDelaySecs = uint64(2)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const BootstrapPeerThreshold = 2

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
// TODO same as butterfly for now, as we didn't submit an assignment for interopnet.
const Eip155ChainId = 3141592

var WhitelistedBlock = cid.Undef

const F3Enabled = true

var F3ManifestServerID = MustParseID("12D3KooWQJ2rdVnG4okDUB6yHQhAjNutGNemcM7XzqC9Eo4z9Jce")

// The initial F3 power table CID.
var F3InitialPowerTableCID cid.Cid = cid.Undef

const F3BootstrapEpoch abi.ChainEpoch = 1000
