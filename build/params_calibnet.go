package build

import (
	"github.com/filecoin-project/go-state-types/abi"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	"github.com/ipfs/go-cid"
)

var calibrationDrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

const calibrationBootstrappersFile = "calibnet.pi"
const calibrationGenesisFile = "calibnet.car"

const calibrationUpgradeBreezeHeight = -1
const calibrationBreezeGasTampingDuration = 120

const calibrationUpgradeSmokeHeight = -2

const calibrationUpgradeIgnitionHeight = -3
const calibrationUpgradeRefuelHeight = -4

var calibrationUpgradeActorsV2Height = abi.ChainEpoch(30)

const calibrationUpgradeTapeHeight = 60

const calibrationUpgradeLiftoffHeight = -5

const calibrationUpgradeKumquatHeight = 90

const calibrationUpgradeCalicoHeight = 100
const calibrationUpgradePersianHeight = calibrationUpgradeCalicoHeight + (builtin2.EpochsInHour * 1)

const calibrationUpgradeClausHeight = 250

const calibrationUpgradeOrangeHeight = 300

const calibrationUpgradeActorsV3Height = 600
const calibrationUpgradeNorwegianHeight = 114000

const calibrationUpgradeActorsV4Height = 193789

const calibrationBlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const calibrationPropagationDelaySecs = uint64(6)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const calibrationBootstrapPeerThreshold = 4

var calibrationWhitelistedBlock = cid.Undef

type calibrationConfigurableParams struct{}

func (calibrationConfigurableParams) DrandSchedule() map[abi.ChainEpoch]DrandEnum {
	return calibrationDrandSchedule
}
func (calibrationConfigurableParams) BootstrappersFile() string {
	return calibrationBootstrappersFile
}
func (calibrationConfigurableParams) GenesisFile() string {
	return calibrationGenesisFile
}
func (calibrationConfigurableParams) UpgradeBreezeHeight() abi.ChainEpoch {
	return calibrationUpgradeBreezeHeight
}
func (calibrationConfigurableParams) BreezeGasTampingDuration() abi.ChainEpoch {
	return calibrationBreezeGasTampingDuration
}
func (calibrationConfigurableParams) UpgradeSmokeHeight() abi.ChainEpoch {
	return calibrationUpgradeSmokeHeight
}
func (calibrationConfigurableParams) UpgradeIgnitionHeight() abi.ChainEpoch {
	return calibrationUpgradeIgnitionHeight
}
func (calibrationConfigurableParams) UpgradeRefuelHeight() abi.ChainEpoch {
	return calibrationUpgradeRefuelHeight
}
func (calibrationConfigurableParams) UpgradeActorsV2Height() abi.ChainEpoch {
	return calibrationUpgradeActorsV2Height
}
func (calibrationConfigurableParams) UpgradeTapeHeight() abi.ChainEpoch {
	return calibrationUpgradeTapeHeight
}
func (calibrationConfigurableParams) UpgradeLiftoffHeight() abi.ChainEpoch {
	return calibrationUpgradeLiftoffHeight
}
func (calibrationConfigurableParams) UpgradeKumquatHeight() abi.ChainEpoch {
	return calibrationUpgradeKumquatHeight
}
func (calibrationConfigurableParams) UpgradeCalicoHeight() abi.ChainEpoch {
	return calibrationUpgradeCalicoHeight
}
func (calibrationConfigurableParams) UpgradePersianHeight() abi.ChainEpoch {
	return calibrationUpgradePersianHeight
}
func (calibrationConfigurableParams) UpgradeOrangeHeight() abi.ChainEpoch {
	return calibrationUpgradeOrangeHeight
}
func (calibrationConfigurableParams) UpgradeClausHeight() abi.ChainEpoch {
	return calibrationUpgradeClausHeight
}
func (calibrationConfigurableParams) UpgradeActorsV3Height() abi.ChainEpoch {
	return calibrationUpgradeActorsV3Height
}
func (calibrationConfigurableParams) UpgradeNorwegianHeight() abi.ChainEpoch {
	return calibrationUpgradeNorwegianHeight
}
func (calibrationConfigurableParams) UpgradeActorsV4Height() abi.ChainEpoch {
	return calibrationUpgradeActorsV4Height
}
func (calibrationConfigurableParams) BlockDelaySecs() uint64 {
	return calibrationBlockDelaySecs
}
func (calibrationConfigurableParams) PropagationDelaySecs() uint64 {
	return calibrationPropagationDelaySecs
}
func (calibrationConfigurableParams) BootstrapPeerThreshold() int {
	return calibrationBootstrapPeerThreshold
}
func (calibrationConfigurableParams) WhitelistedBlock() cid.Cid {
	return calibrationWhitelistedBlock
}
func (calibrationConfigurableParams) InsecurePoStValidation() bool {
	return false
}
func (calibrationConfigurableParams) Devnet() bool {
	return true
}
