//go:build !debug && !2k && !testground && !calibnet && !butterflynet && !interopnet

package buildconstants

import (
	_ "embed"
	"math"
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
	0:                    DrandIncentinet,
	UpgradeSmokeHeight:   DrandMainnet,
	UpgradePhoenixHeight: DrandQuicknet,
}

var NetworkBundle = "mainnet"

// NOTE: DO NOT change this unless you REALLY know what you're doing. This is consensus critical.
const ActorDebugging = false

const GenesisNetworkVersion = network.Version0

const BootstrappersFile = "mainnet.pi"
const GenesisFile = "mainnet.car.zst"

const UpgradeBreezeHeight abi.ChainEpoch = 41280

const BreezeGasTampingDuration abi.ChainEpoch = 120

const UpgradeSmokeHeight abi.ChainEpoch = 51000

const UpgradeIgnitionHeight abi.ChainEpoch = 94000
const UpgradeRefuelHeight abi.ChainEpoch = 130800

var UpgradeAssemblyHeight abi.ChainEpoch = 138720

const UpgradeTapeHeight abi.ChainEpoch = 140760

// This signals our tentative epoch for mainnet launch. Can make it later, but not earlier.
// Miners, clients, developers, custodians all need time to prepare.
// We still have upgrades and state changes to do, but can happen after signaling timing here.
const UpgradeLiftoffHeight abi.ChainEpoch = 148888

const UpgradeKumquatHeight abi.ChainEpoch = 170000

const UpgradeCalicoHeight abi.ChainEpoch = 265200
const UpgradePersianHeight abi.ChainEpoch = UpgradeCalicoHeight + (builtin2.EpochsInHour * 60)

const UpgradeOrangeHeight abi.ChainEpoch = 336458

// 2020-12-22T02:00:00Z
// var because of wdpost_test.go
var UpgradeClausHeight abi.ChainEpoch = 343200

// 2021-03-04T00:00:30Z
const UpgradeTrustHeight abi.ChainEpoch = 550321

// 2021-04-12T22:00:00Z
const UpgradeNorwegianHeight abi.ChainEpoch = 665280

// 2021-04-29T06:00:00Z
const UpgradeTurboHeight abi.ChainEpoch = 712320

// 2021-06-30T22:00:00Z
const UpgradeHyperdriveHeight abi.ChainEpoch = 892800

// 2021-10-26T13:30:00Z
const UpgradeChocolateHeight abi.ChainEpoch = 1231620

// 2022-03-01T15:00:00Z
const UpgradeOhSnapHeight abi.ChainEpoch = 1594680

// 2022-07-06T14:00:00Z
const UpgradeSkyrHeight abi.ChainEpoch = 1960320

// 2022-11-30T14:00:00Z
const UpgradeSharkHeight abi.ChainEpoch = 2383680

// 2023-03-14T15:14:00Z
const UpgradeHyggeHeight abi.ChainEpoch = 2683348

// 2023-04-27T13:00:00Z
const UpgradeLightningHeight abi.ChainEpoch = 2809800

// 2023-05-18T13:00:00Z
const UpgradeThunderHeight abi.ChainEpoch = UpgradeLightningHeight + 2880*21

// 2023-12-12T13:30:00Z
const UpgradeWatermelonHeight abi.ChainEpoch = 3469380

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFixHeight abi.ChainEpoch = -1

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFix2Height abi.ChainEpoch = -2

// 2024-04-24T14:00:00Z
const UpgradeDragonHeight abi.ChainEpoch = 3855360

// This fix upgrade only ran on calibrationnet
const UpgradeCalibrationDragonFixHeight abi.ChainEpoch = -3

// This epoch, 120 epochs after the "rest" of the nv22 upgrade, is when we switch to Drand quicknet
// 2024-04-11T15:00:00Z
const UpgradePhoenixHeight abi.ChainEpoch = UpgradeDragonHeight + 120

// 2024-08-06T12:00:00Z
const UpgradeWaffleHeight abi.ChainEpoch = 4154640

// 2024-11-20T23:00:00Z
// var because of TestMigrationNV24 in itests/migration_test.go to test the FIP-0081 pledge ramp
var UpgradeTuktukHeight abi.ChainEpoch = 4461240

// FIP-0081: for the power actor state for pledge calculations.
// UpgradeTuktukPowerRampDurationEpochs ends up in the power actor state after
// Tuktuk migration. along with a RampStartEpoch matching the upgrade height.
var UpgradeTuktukPowerRampDurationEpochs = uint64(builtin.EpochsInYear)

// 2025-04-14T23:00:00Z
var UpgradeTeepHeight = abi.ChainEpoch(4878840)

// This epoch, 90 days after Teep is the completion of FIP-0100 where actors will start applying
// the new daily fee to pre-Teep sectors being extended.
var UpgradeTockHeight = UpgradeTeepHeight + builtin.EpochsInDay*90

// Only applied to calibnet which was already upgraded to Teep&Tock
var UpgradeTockFixHeight = abi.ChainEpoch(-1)

// 2025-09-24T23:00:00Z
const UpgradeGoldenWeekHeight = abi.ChainEpoch(5348280)

// ????
var UpgradeXxHeight = abi.ChainEpoch(9999999999)

var UpgradeTeepInitialFilReserved = InitialFilReserved // FIP-0100: no change for mainnet

var ConsensusMinerMinPower = abi.NewStoragePower(10 << 40)
var PreCommitChallengeDelay = abi.ChainEpoch(150)
var PropagationDelaySecs = uint64(10)

var EquivocationDelaySecs = uint64(2)

func init() {
	var addrNetwork address.Network
	if os.Getenv("LOTUS_USE_TEST_ADDRESSES") != "1" {
		addrNetwork = address.Mainnet
	} else {
		addrNetwork = address.Testnet
	}
	SetAddressNetwork(addrNetwork)

	if os.Getenv("LOTUS_DISABLE_XX") == "1" {
		UpgradeXxHeight = math.MaxInt64 - 1
	}

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

	Devnet = false

	BuildType = BuildMainnet
}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const BootstrapPeerThreshold = 4

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = 314

// WhitelistedBlock skips checks on message validity in this block to sidestep the zero-bls signature
var WhitelistedBlock = cid.MustParse("bafy2bzaceapyg2uyzk7vueh3xccxkuwbz3nxewjyguoxvhx77malc2lzn2ybi")

const F3Enabled = true

//go:embed f3manifest_mainnet.json
var F3ManifestBytes []byte
