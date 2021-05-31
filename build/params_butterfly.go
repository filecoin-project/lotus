package build

import (
	"github.com/filecoin-project/go-state-types/abi"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	"github.com/ipfs/go-cid"
)

var butterflyDrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

const butterflyBootstrappersFile = "butterflynet.pi"
const butterflyGenesisFile = "butterflynet.car"

const butterflyUpgradeBreezeHeight = -1
const butterflyBreezeGasTampingDuration = 120
const butterflyUpgradeSmokeHeight = -2
const butterflyUpgradeIgnitionHeight = -3
const butterflyUpgradeRefuelHeight = -4

var butterflyUpgradeActorsV2Height = abi.ChainEpoch(30)

const butterflyUpgradeTapeHeight = 60
const butterflyUpgradeLiftoffHeight = -5
const butterflyUpgradeKumquatHeight = 90
const butterflyUpgradeCalicoHeight = 120
const butterflyUpgradePersianHeight = 150
const butterflyUpgradeClausHeight = 180
const butterflyUpgradeOrangeHeight = 210
const butterflyUpgradeActorsV3Height = 240
const butterflyUpgradeNorwegianHeight = butterflyUpgradeActorsV3Height + (builtin2.EpochsInHour * 12)
const butterflyUpgradeActorsV4Height = 8922

const butterflyBlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const butterflyPropagationDelaySecs = uint64(6)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const butterflyBootstrapPeerThreshold = 2

var butterflyWhitelistedBlock = cid.Undef

type butterflyConfigurableParams struct{}

func (butterflyConfigurableParams) DrandSchedule() map[abi.ChainEpoch]DrandEnum {
	return butterflyDrandSchedule
}
func (butterflyConfigurableParams) BootstrappersFile() string {
	return butterflyBootstrappersFile
}
func (butterflyConfigurableParams) GenesisFile() string {
	return butterflyGenesisFile
}
func (butterflyConfigurableParams) UpgradeBreezeHeight() abi.ChainEpoch {
	return butterflyUpgradeBreezeHeight
}
func (butterflyConfigurableParams) BreezeGasTampingDuration() abi.ChainEpoch {
	return butterflyBreezeGasTampingDuration
}
func (butterflyConfigurableParams) UpgradeSmokeHeight() abi.ChainEpoch {
	return butterflyUpgradeSmokeHeight
}
func (butterflyConfigurableParams) UpgradeIgnitionHeight() abi.ChainEpoch {
	return butterflyUpgradeIgnitionHeight
}
func (butterflyConfigurableParams) UpgradeRefuelHeight() abi.ChainEpoch {
	return butterflyUpgradeRefuelHeight
}
func (butterflyConfigurableParams) UpgradeActorsV2Height() abi.ChainEpoch {
	return butterflyUpgradeActorsV2Height
}
func (butterflyConfigurableParams) UpgradeTapeHeight() abi.ChainEpoch {
	return butterflyUpgradeTapeHeight
}
func (butterflyConfigurableParams) UpgradeLiftoffHeight() abi.ChainEpoch {
	return butterflyUpgradeLiftoffHeight
}
func (butterflyConfigurableParams) UpgradeKumquatHeight() abi.ChainEpoch {
	return butterflyUpgradeKumquatHeight
}
func (butterflyConfigurableParams) UpgradeCalicoHeight() abi.ChainEpoch {
	return butterflyUpgradeCalicoHeight
}
func (butterflyConfigurableParams) UpgradePersianHeight() abi.ChainEpoch {
	return butterflyUpgradePersianHeight
}
func (butterflyConfigurableParams) UpgradeOrangeHeight() abi.ChainEpoch {
	return butterflyUpgradeOrangeHeight
}
func (butterflyConfigurableParams) UpgradeClausHeight() abi.ChainEpoch {
	return butterflyUpgradeClausHeight
}
func (butterflyConfigurableParams) UpgradeActorsV3Height() abi.ChainEpoch {
	return butterflyUpgradeActorsV3Height
}
func (butterflyConfigurableParams) UpgradeNorwegianHeight() abi.ChainEpoch {
	return butterflyUpgradeNorwegianHeight
}
func (butterflyConfigurableParams) UpgradeActorsV4Height() abi.ChainEpoch {
	return butterflyUpgradeActorsV4Height
}
func (butterflyConfigurableParams) BlockDelaySecs() uint64 {
	return butterflyBlockDelaySecs
}
func (butterflyConfigurableParams) PropagationDelaySecs() uint64 {
	return butterflyPropagationDelaySecs
}
func (butterflyConfigurableParams) BootstrapPeerThreshold() int {
	return butterflyBootstrapPeerThreshold
}
func (butterflyConfigurableParams) WhitelistedBlock() cid.Cid {
	return butterflyWhitelistedBlock
}
func (butterflyConfigurableParams) InsecurePoStValidation() bool {
	return false
}
func (butterflyConfigurableParams) Devnet() bool {
	return true
}
