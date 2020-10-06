// +build debug 2k

package build

import (
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

const UpgradeBreezeHeight = -1
const BreezeGasTampingDuration = 0

const UpgradeSmokeHeight = -1
const UpgradeIgnitionHeight = -2
const UpgradeRefuelHeight = -3
const UpgradeActorsV2Height = 10
const UpgradeLiftoffHeight = -4

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))

	BuildType |= Build2k
}

const BlockDelaySecs = uint64(4)

const PropagationDelaySecs = uint64(1)

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const SlashablePowerDelay = 20

// Epochs
const InteractivePoRepConfidence = 6
