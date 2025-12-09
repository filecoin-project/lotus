//go:build calibnet

package buildconstants

import (
	_ "embed"
	"os"
	"strconv"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0:                    DrandMainnet,
	UpgradePhoenixHeight: DrandQuicknet,
}

const GenesisNetworkVersion = network.Version0

var NetworkBundle = "calibrationnet"
var ActorDebugging = false

const BootstrappersFile = "calibnet.pi"
const GenesisFile = "calibnet.car.zst"

const UpgradeBreezeHeight abi.ChainEpoch = -1
const BreezeGasTampingDuration = 120

const UpgradeSmokeHeight abi.ChainEpoch = -2

const UpgradeIgnitionHeight abi.ChainEpoch = -3
const UpgradeRefuelHeight abi.ChainEpoch = -4

var UpgradeAssemblyHeight = abi.ChainEpoch(30)

const UpgradeTapeHeight abi.ChainEpoch = 60

const UpgradeLiftoffHeight abi.ChainEpoch = -5

const UpgradeKumquatHeight abi.ChainEpoch = 90

const UpgradeCalicoHeight abi.ChainEpoch = 120
const UpgradePersianHeight abi.ChainEpoch = UpgradeCalicoHeight + (builtin2.EpochsInHour * 1)

const UpgradeClausHeight abi.ChainEpoch = 270

const UpgradeOrangeHeight abi.ChainEpoch = 300

const UpgradeTrustHeight abi.ChainEpoch = 330

const UpgradeNorwegianHeight abi.ChainEpoch = 360

const UpgradeTurboHeight abi.ChainEpoch = 390

const UpgradeHyperdriveHeight abi.ChainEpoch = 420

const UpgradeChocolateHeight abi.ChainEpoch = 450

const UpgradeOhSnapHeight abi.ChainEpoch = 480

const UpgradeSkyrHeight abi.ChainEpoch = 510

const UpgradeSharkHeight abi.ChainEpoch = 16800 // 6 days after genesis

// 2023-02-21T16:30:00Z
const UpgradeHyggeHeight abi.ChainEpoch = 322354

// 2023-04-20T14:00:00Z
const UpgradeLightningHeight abi.ChainEpoch = 489094

// 2023-04-21T16:00:00Z
const UpgradeThunderHeight abi.ChainEpoch = UpgradeLightningHeight + 3120

// 2023-10-19T13:00:00Z
const UpgradeWatermelonHeight abi.ChainEpoch = 1013134

// 2023-11-07T13:00:00Z
const UpgradeWatermelonFixHeight abi.ChainEpoch = 1070494

// 2023-11-21T13:00:00Z
const UpgradeWatermelonFix2Height abi.ChainEpoch = 1108174

// 2024-03-11T14:00:00Z
const UpgradeDragonHeight abi.ChainEpoch = 1427974

// This epoch, 120 epochs after the "rest" of the nv22 upgrade, is when we switch to Drand quicknet
const UpgradePhoenixHeight abi.ChainEpoch = UpgradeDragonHeight + 120

// 2024-04-03T11:00:00Z
const UpgradeCalibrationDragonFixHeight abi.ChainEpoch = 1493854

// 2024-07-11T12:00:00Z
const UpgradeWaffleHeight abi.ChainEpoch = 1779094

// 2024-10-23T13:30:00Z
const UpgradeTuktukHeight abi.ChainEpoch = 2078794

// FIP-0081: for the power actor state for pledge calculations.
// UpgradeTuktukPowerRampDurationEpochs ends up in the power actor state after
// Tuktuk migration. along with a RampStartEpoch matching the upgrade height.
//
// For calibrationnet, we set this to 3 days so we can observe and confirm the
// ramp behavior before mainnet upgrade.
var UpgradeTuktukPowerRampDurationEpochs = uint64(builtin.EpochsInDay * 3)

// 2025-03-26T23:00:00Z
// Calibnet was upgraded at this height but a fix needed to be applied, this was
// done at UpgradeTockFixHeight.
const UpgradeTeepHeight abi.ChainEpoch = 2523454

var UpgradeTeepInitialFilReserved = wholeFIL(1_200_000_000) // FIP-0100: 300M -> 1.2B FIL

// This epoch, 7 days after Teep is the completion of FIP-0100 where actors will start applying
// the new daily fee to pre-Teep sectors being extended. This is 90 days on mainnet.
var UpgradeTockHeight abi.ChainEpoch = UpgradeTeepHeight + builtin.EpochsInDay*7

// 2025-04-07T23:00:00Z
const UpgradeTockFixHeight abi.ChainEpoch = 2558014

// 2025-09-10T23:00:00Z
const UpgradeGoldenWeekHeight abi.ChainEpoch = 3007294

// ??????
const UpgradeXxHeight = 999999999999999

var ConsensusMinerMinPower = abi.NewStoragePower(32 << 30)
var PreCommitChallengeDelay = abi.ChainEpoch(150)

func init() {
	SetAddressNetwork(address.Testnet)

	Devnet = true

	// NOTE: DO NOT change this unless you REALLY know what you're doing. This is not consensus critical, however,
	//set this value too high may impacts your block submission; set this value too low may cause you miss
	//parent tipsets for blocking forming and mining.
	if len(os.Getenv("PROPAGATION_DELAY_SECS")) != 0 {
		pds, err := strconv.ParseUint(os.Getenv("PROPAGATION_DELAY_SECS"), 10, 64)
		if err != nil {
			log.Warnw("Error setting PROPAGATION_DELAY_SECS, %v, proceed with default value %s", err,
				PropagationDelaySecs)
		} else {
			PropagationDelaySecs = pds
			log.Warnw(" !!WARNING!! propagation delay is set to be %s second, "+
				"this value impacts your message republish interval and block forming - monitor with caution!!", PropagationDelaySecs)
		}
	}

	BuildType = BuildCalibnet
}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

var PropagationDelaySecs = uint64(10)

var EquivocationDelaySecs = uint64(2)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const BootstrapPeerThreshold = 4

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = 314159

var WhitelistedBlock = cid.Undef

const F3Enabled = true

//go:embed f3manifest_calibnet.json
var F3ManifestBytes []byte
