//go:build !debug && !2k && !testground && !calibnet && !nerpanet && !butterflynet && !interopnet
// +build !debug,!2k,!testground,!calibnet,!nerpanet,!butterflynet,!interopnet

package build

import (
	"math"
	"os"

	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
)

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0:                  DrandIncentinet,
	UpgradeSmokeHeight: DrandMainnet,
}

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
var UpgradeHyperdriveHeight = abi.ChainEpoch(892800)

// 2021-10-26T13:30:00Z
var UpgradeChocolateHeight = abi.ChainEpoch(1231620)

func init() {
	if os.Getenv("LOTUS_USE_TEST_ADDRESSES") != "1" {
		SetAddressNetwork(address.Mainnet)
	}

	if os.Getenv("LOTUS_DISABLE_CHOCOLATE") == "1" {
		UpgradeChocolateHeight = math.MaxInt64
	}

	Devnet = false

	BuildType = BuildMainnet
}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const BootstrapPeerThreshold = 4

// we skip checks on message validity in this block to sidestep the zero-bls signature
var WhitelistedBlock = MustParseCid("bafy2bzaceapyg2uyzk7vueh3xccxkuwbz3nxewjyguoxvhx77malc2lzn2ybi")
