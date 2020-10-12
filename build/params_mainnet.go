// +build !debug
// +build !2k
// +build !testground

package build

import (
	"math"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/policy"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
)

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0:                  DrandIncentinet,
	UpgradeSmokeHeight: DrandMainnet,
}

const UpgradeBreezeHeight = 41280
const BreezeGasTampingDuration = 120

const UpgradeSmokeHeight = 51000

const UpgradeIgnitionHeight = 94000
const UpgradeRefuelHeight = 130800

var UpgradeActorsV2Height = abi.ChainEpoch(138720)

const UpgradeTapeHeight = 140760

// This signals our tentative epoch for mainnet launch. Can make it later, but not earlier.
// Miners, clients, developers, custodians all need time to prepare.
// We still have upgrades and state changes to do, but can happen after signaling timing here.
const UpgradeLiftoffHeight = 148888

func init() {
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(10 << 40))
	policy.SetSupportedProofTypes(
		abi.RegisteredSealProof_StackedDrg32GiBV1,
		abi.RegisteredSealProof_StackedDrg64GiBV1,
	)

	if os.Getenv("LOTUS_USE_TEST_ADDRESSES") != "1" {
		SetAddressNetwork(address.Mainnet)
	}

	if os.Getenv("LOTUS_DISABLE_V2_ACTOR_MIGRATION") == "1" {
		UpgradeActorsV2Height = math.MaxInt64
	}

	Devnet = false
}

const BlockDelaySecs = uint64(builtin0.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)
