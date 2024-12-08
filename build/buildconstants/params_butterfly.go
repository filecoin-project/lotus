//go:build butterflynet
// +build butterflynet

package buildconstants

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandQuicknet,
}

const GenesisNetworkVersion = network.Version24

var NetworkBundle = "butterflynet"
var ActorDebugging = false

const BootstrappersFile = "butterflynet.pi"
const GenesisFile = "butterflynet.car.zst"

const UpgradeBreezeHeight = -1
const BreezeGasTampingDuration = 120
const UpgradeSmokeHeight = -2
const UpgradeIgnitionHeight = -3
const UpgradeRefuelHeight = -4

var UpgradeAssemblyHeight = abi.ChainEpoch(-5)

const UpgradeTapeHeight = -6
const UpgradeLiftoffHeight = -7
const UpgradeKumquatHeight = -8
const UpgradeCalicoHeight = -9
const UpgradePersianHeight = -10
const UpgradeClausHeight = -11
const UpgradeOrangeHeight = -12
const UpgradeTrustHeight = -13
const UpgradeNorwegianHeight = -14
const UpgradeTurboHeight = -15
const UpgradeHyperdriveHeight = -16
const UpgradeChocolateHeight = -17
const UpgradeOhSnapHeight = -18
const UpgradeSkyrHeight = -19
const UpgradeSharkHeight = -20
const UpgradeHyggeHeight = -21
const UpgradeLightningHeight = -22
const UpgradeThunderHeight = -23
const UpgradeWatermelonHeight = -24
const UpgradeDragonHeight = -25
const UpgradePhoenixHeight = -26
const UpgradeWaffleHeight = -27
const UpgradeTuktukHeight = -28

// ??????
const UpgradeTeepHeight = 100

// FIP-0081: for the power actor state for pledge calculations.
// UpgradeTuktukPowerRampDurationEpochs ends up in the power actor state after
// Tuktuk migration. along with a RampStartEpoch matching the upgrade height.
var UpgradeTuktukPowerRampDurationEpochs = uint64(builtin.EpochsInYear)

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFixHeight = -100

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFix2Height = -101

// This fix upgrade only ran on calibrationnet
const UpgradeCalibrationDragonFixHeight = -102

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg512MiBV1,
	abi.RegisteredSealProof_StackedDrg32GiBV1,
	abi.RegisteredSealProof_StackedDrg64GiBV1,
}
var ConsensusMinerMinPower = abi.NewStoragePower(2 << 30)
var PreCommitChallengeDelay = abi.ChainEpoch(150)

func init() {
	SetAddressNetwork(address.Testnet)

	Devnet = true

	BuildType = BuildButterflynet
}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)

var EquivocationDelaySecs = uint64(2)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const BootstrapPeerThreshold = 2

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = 3141592

var WhitelistedBlock = cid.Undef

const F3Enabled = true

var F3ManifestServerID = MustParseID("12D3KooWJr9jy4ngtJNR7JC1xgLFra3DjEtyxskRYWvBK9TC3Yn6")

// The initial F3 power table CID.
var F3InitialPowerTableCID cid.Cid = cid.Undef

const F3BootstrapEpoch abi.ChainEpoch = 1000
