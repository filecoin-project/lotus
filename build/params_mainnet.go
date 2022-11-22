//go:build !debug && !2k && !testground && !calibnet && !butterflynet && !interopnet
// +build !debug,!2k,!testground,!calibnet,!butterflynet,!interopnet

package build

import (
	"math"
	"os"
	"strconv"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/network"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
)

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0:                  DrandIncentinet,
	UpgradeSmokeHeight: DrandMainnet,
}

var NetworkBundle = "mainnet"

// NOTE: DO NOT change this unless you REALLY know what you're doing. This is consensus critical.
var BundleOverrides map[actorstypes.Version]string

const GenesisNetworkVersion = network.Version0

const BootstrappersFile = "mainnet.pi"
const GenesisFile = "mainnet.car"

const UpgradeBreezeHeight = 41280

const BreezeGasTampingDuration = 120

const UpgradeSmokeHeight = 51000

const UpgradeIgnitionHeight = 94000
const UpgradeRefuelHeight = 130800

const UpgradeAssemblyHeight = 138720

const UpgradeTapeHeight = 140760

// This signals our tentative epoch for mainnet launch. Can make it later, but not earlier.
// Miners, clients, developers, custodians all need time to prepare.
// We still have upgrades and state changes to do, but can happen after signaling timing here.
const UpgradeLiftoffHeight = 148888

const UpgradeKumquatHeight = 170000

const UpgradeCalicoHeight = 265200
const UpgradePersianHeight = UpgradeCalicoHeight + (builtin2.EpochsInHour * 60)

const UpgradeOrangeHeight = 336458

// 2020-12-22T02:00:00Z
var UpgradeClausHeight = abi.ChainEpoch(343200)

// 2021-03-04T00:00:30Z
const UpgradeTrustHeight = 550321

// 2021-04-12T22:00:00Z
const UpgradeNorwegianHeight = 665280

// 2021-04-29T06:00:00Z
const UpgradeTurboHeight = 712320

// 2021-06-30T22:00:00Z
const UpgradeHyperdriveHeight = 892800

// 2021-10-26T13:30:00Z
const UpgradeChocolateHeight = 1231620

// 2022-03-01T15:00:00Z
const UpgradeOhSnapHeight = 1594680

// 2022-07-06T14:00:00Z
const UpgradeSkyrHeight = 1960320

// 2022-11-30T14:00:00Z
var UpgradeSharkHeight = abi.ChainEpoch(2383680)

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg32GiBV1,
	abi.RegisteredSealProof_StackedDrg64GiBV1,
}
var ConsensusMinerMinPower = abi.NewStoragePower(10 << 40)
var MinVerifiedDealSize = abi.NewStoragePower(1 << 20)
var PreCommitChallengeDelay = abi.ChainEpoch(150)
var PropagationDelaySecs = uint64(10)

func init() {
	if os.Getenv("LOTUS_USE_TEST_ADDRESSES") != "1" {
		SetAddressNetwork(address.Mainnet)
	}

	if os.Getenv("LOTUS_DISABLE_SHARK") == "1" {
		UpgradeSharkHeight = math.MaxInt64
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

// we skip checks on message validity in this block to sidestep the zero-bls signature
var WhitelistedBlock = MustParseCid("bafy2bzaceapyg2uyzk7vueh3xccxkuwbz3nxewjyguoxvhx77malc2lzn2ybi")
