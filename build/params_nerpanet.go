package build

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
)

var nerpaDrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

const nerpaBootstrappersFile = "nerpanet.pi"
const nerpaGenesisFile = "nerpanet.car"

const nerpaUpgradeBreezeHeight = -1
const nerpaBreezeGasTampingDuration = 0

const nerpaUpgradeSmokeHeight = -1

const nerpaUpgradeIgnitionHeight = -2
const nerpaUpgradeRefuelHeight = -3

const nerpaUpgradeLiftoffHeight = -5

const nerpaUpgradeActorsV2Height = 30 // critical: the network can bootstrap from v1 only
const nerpaUpgradeTapeHeight = 60

const nerpaUpgradeKumquatHeight = 90

const nerpaUpgradeCalicoHeight = 100
const nerpaUpgradePersianHeight = nerpaUpgradeCalicoHeight + (builtin2.EpochsInHour * 1)

const nerpaUpgradeClausHeight = 250

const nerpaUpgradeOrangeHeight = 300

const nerpaUpgradeActorsV3Height = 600
const nerpaUpgradeNorwegianHeight = 201000
const nerpaUpgradeActorsV4Height = 203000

const nerpaBlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const nerpaPropagationDelaySecs = uint64(6)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const nerpaBootstrapPeerThreshold = 4

var nerpaWhitelistedBlock = cid.Undef

type nerpaConfigurableParams struct{}

func (nerpaConfigurableParams) DrandSchedule() map[abi.ChainEpoch]DrandEnum {
	return nerpaDrandSchedule
}
func (nerpaConfigurableParams) BootstrappersFile() string {
	return nerpaBootstrappersFile
}
func (nerpaConfigurableParams) GenesisFile() string {
	return nerpaGenesisFile
}
func (nerpaConfigurableParams) UpgradeBreezeHeight() abi.ChainEpoch {
	return nerpaUpgradeBreezeHeight
}
func (nerpaConfigurableParams) BreezeGasTampingDuration() abi.ChainEpoch {
	return nerpaBreezeGasTampingDuration
}
func (nerpaConfigurableParams) UpgradeSmokeHeight() abi.ChainEpoch {
	return nerpaUpgradeSmokeHeight
}
func (nerpaConfigurableParams) UpgradeIgnitionHeight() abi.ChainEpoch {
	return nerpaUpgradeIgnitionHeight
}
func (nerpaConfigurableParams) UpgradeRefuelHeight() abi.ChainEpoch {
	return nerpaUpgradeRefuelHeight
}
func (nerpaConfigurableParams) UpgradeActorsV2Height() abi.ChainEpoch {
	return nerpaUpgradeActorsV2Height
}
func (nerpaConfigurableParams) UpgradeTapeHeight() abi.ChainEpoch {
	return nerpaUpgradeTapeHeight
}
func (nerpaConfigurableParams) UpgradeLiftoffHeight() abi.ChainEpoch {
	return nerpaUpgradeLiftoffHeight
}
func (nerpaConfigurableParams) UpgradeKumquatHeight() abi.ChainEpoch {
	return nerpaUpgradeKumquatHeight
}
func (nerpaConfigurableParams) UpgradeCalicoHeight() abi.ChainEpoch {
	return nerpaUpgradeCalicoHeight
}
func (nerpaConfigurableParams) UpgradePersianHeight() abi.ChainEpoch {
	return nerpaUpgradePersianHeight
}
func (nerpaConfigurableParams) UpgradeOrangeHeight() abi.ChainEpoch {
	return nerpaUpgradeOrangeHeight
}
func (nerpaConfigurableParams) UpgradeClausHeight() abi.ChainEpoch {
	return nerpaUpgradeClausHeight
}
func (nerpaConfigurableParams) UpgradeActorsV3Height() abi.ChainEpoch {
	return nerpaUpgradeActorsV3Height
}
func (nerpaConfigurableParams) UpgradeNorwegianHeight() abi.ChainEpoch {
	return nerpaUpgradeNorwegianHeight
}
func (nerpaConfigurableParams) UpgradeActorsV4Height() abi.ChainEpoch {
	return nerpaUpgradeActorsV4Height
}
func (nerpaConfigurableParams) BlockDelaySecs() uint64 {
	return nerpaBlockDelaySecs
}
func (nerpaConfigurableParams) PropagationDelaySecs() uint64 {
	return nerpaPropagationDelaySecs
}
func (nerpaConfigurableParams) BootstrapPeerThreshold() int {
	return nerpaBootstrapPeerThreshold
}
func (nerpaConfigurableParams) WhitelistedBlock() cid.Cid {
	return nerpaWhitelistedBlock
}
func (nerpaConfigurableParams) InsecurePoStValidation() bool {
	return false
}
func (nerpaConfigurableParams) Devnet() bool {
	return false
}
