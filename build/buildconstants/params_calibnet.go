//go:build calibnet
// +build calibnet

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

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0:                    DrandMainnet,
	UpgradePhoenixHeight: DrandQuicknet,
}

const GenesisNetworkVersion = network.Version0

var NetworkBundle = "calibrationnet"
var ActorDebugging = false

const BootstrappersFile = "calibnet.pi"
const GenesisFile = "calibnet.car.zst"

const UpgradeBreezeHeight = -1
const BreezeGasTampingDuration = 120

const UpgradeSmokeHeight = -2

const UpgradeIgnitionHeight = -3
const UpgradeRefuelHeight = -4

var UpgradeAssemblyHeight = abi.ChainEpoch(30)

const UpgradeTapeHeight = 60

const UpgradeLiftoffHeight = -5

const UpgradeKumquatHeight = 90

const UpgradeCalicoHeight = 120
const UpgradePersianHeight = UpgradeCalicoHeight + (builtin2.EpochsInHour * 1)

const UpgradeClausHeight = 270

const UpgradeOrangeHeight = 300

const UpgradeTrustHeight = 330

const UpgradeNorwegianHeight = 360

const UpgradeTurboHeight = 390

const UpgradeHyperdriveHeight = 420

const UpgradeChocolateHeight = 450

const UpgradeOhSnapHeight = 480

const UpgradeSkyrHeight = 510

const UpgradeSharkHeight = 16800 // 6 days after genesis

// 2023-02-21T16:30:00Z
const UpgradeHyggeHeight = 322354

// 2023-04-20T14:00:00Z
const UpgradeLightningHeight = 489094

// 2023-04-21T16:00:00Z
const UpgradeThunderHeight = UpgradeLightningHeight + 3120

// 2023-10-19T13:00:00Z
const UpgradeWatermelonHeight = 1013134

// 2023-11-07T13:00:00Z
const UpgradeWatermelonFixHeight = 1070494

// 2023-11-21T13:00:00Z
const UpgradeWatermelonFix2Height = 1108174

// 2024-03-11T14:00:00Z
const UpgradeDragonHeight = 1427974

// This epoch, 120 epochs after the "rest" of the nv22 upgrade, is when we switch to Drand quicknet
const UpgradePhoenixHeight = UpgradeDragonHeight + 120

// 2024-04-03T11:00:00Z
const UpgradeCalibrationDragonFixHeight = 1493854

// 2024-07-11T12:00:00Z
const UpgradeWaffleHeight = 1779094

// 2024-10-23T13:30:00Z
const UpgradeTuktukHeight = 2078794

// Canceled - See update in: https://github.com/filecoin-project/community/discussions/74#discussioncomment-11549619
const UpgradeTeepHeight = 9999999999

// FIP-0081: for the power actor state for pledge calculations.
// UpgradeTuktukPowerRampDurationEpochs ends up in the power actor state after
// Tuktuk migration. along with a RampStartEpoch matching the upgrade height.
//
// For calibrationnet, we set this to 3 days so we can observe and confirm the
// ramp behavior before mainnet upgrade.
var UpgradeTuktukPowerRampDurationEpochs = uint64(builtin.EpochsInDay * 3)

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg32GiBV1,
	abi.RegisteredSealProof_StackedDrg64GiBV1,
}
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

var F3ManifestServerID = MustParseID("12D3KooWS9vD9uwm8u2uPyJV32QBAhKAmPYwmziAgr3Xzk2FU1Mr")

// The initial F3 power table CID.
var F3InitialPowerTableCID cid.Cid = cid.MustParse("bafy2bzaceab236vmmb3n4q4tkvua2n4dphcbzzxerxuey3mot4g3cov5j3r2c")

// Calibnet F3 activation epoch is 2024-10-24T13:30:00Z - Epoch 2081674
const F3BootstrapEpoch abi.ChainEpoch = UpgradeTuktukHeight + 2880
