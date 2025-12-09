//go:build butterflynet

package buildconstants

import (
	_ "embed"

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

const GenesisNetworkVersion = network.Version27

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

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFixHeight = -100

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFix2Height = -101
const UpgradeDragonHeight = -25

// This fix upgrade only ran on calibrationnet
const UpgradeCalibrationDragonFixHeight = -102
const UpgradePhoenixHeight = -26
const UpgradeWaffleHeight = -27
const UpgradeTuktukHeight = -28

// FIP-0081: for the power actor state for pledge calculations.
// UpgradeTuktukPowerRampDurationEpochs ends up in the power actor state after
// Tuktuk migration. along with a RampStartEpoch matching the upgrade height.
var UpgradeTuktukPowerRampDurationEpochs = uint64(builtin.EpochsInYear)

const UpgradeTeepHeight = -29

var UpgradeTeepInitialFilReserved = wholeFIL(1_600_000_000) // FIP-0100: 300M -> 1.6B FIL

const UpgradeTockHeight = -30

// This fix upgrade only ran on calibrationnet
const UpgradeTockFixHeight = -103

var UpgradeGoldenWeekHeight = abi.ChainEpoch(-31)

// ??????
const UpgradeXxHeight = 999999999999999

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

//go:embed f3manifest_butterfly.json
var F3ManifestBytes []byte
