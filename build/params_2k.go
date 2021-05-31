package build

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
)

const twokBootstrappersFile = ""
const twokGenesisFile = ""

var twokUpgradeBreezeHeight = abi.ChainEpoch(-1)

const twokBreezeGasTampingDuration = 0

var twokUpgradeSmokeHeight = abi.ChainEpoch(-1)
var twokUpgradeIgnitionHeight = abi.ChainEpoch(-2)
var twokUpgradeRefuelHeight = abi.ChainEpoch(-3)
var twokUpgradeTapeHeight = abi.ChainEpoch(-4)

var twokUpgradeActorsV2Height = abi.ChainEpoch(-5)
var twokUpgradeLiftoffHeight = abi.ChainEpoch(-6)

var twokUpgradeKumquatHeight = abi.ChainEpoch(-7)
var twokUpgradeCalicoHeight = abi.ChainEpoch(-8)
var twokUpgradePersianHeight = abi.ChainEpoch(-9)
var twokUpgradeOrangeHeight = abi.ChainEpoch(-10)
var twokUpgradeClausHeight = abi.ChainEpoch(-11)

var twokUpgradeActorsV3Height = abi.ChainEpoch(-12)

var twokUpgradeNorwegianHeight = abi.ChainEpoch(-13)

var twokUpgradeActorsV4Height = abi.ChainEpoch(-14)

var twokDrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

const twokBlockDelaySecs = uint64(4)

const twokPropagationDelaySecs = uint64(1)

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const twokSlashablePowerDelay = 20

// Epochs
const twokInteractivePoRepConfidence = 6

const twokBootstrapPeerThreshold = 1

var twokWhitelistedBlock = cid.Undef

type twokConfigurableParams struct{}

func (twokConfigurableParams) DrandSchedule() map[abi.ChainEpoch]DrandEnum {
	return twokDrandSchedule
}
func (twokConfigurableParams) BootstrappersFile() string {
	return twokBootstrappersFile
}
func (twokConfigurableParams) GenesisFile() string {
	return twokGenesisFile
}
func (twokConfigurableParams) UpgradeBreezeHeight() abi.ChainEpoch {
	return twokUpgradeBreezeHeight
}
func (twokConfigurableParams) BreezeGasTampingDuration() abi.ChainEpoch {
	return twokBreezeGasTampingDuration
}
func (twokConfigurableParams) UpgradeSmokeHeight() abi.ChainEpoch {
	return twokUpgradeSmokeHeight
}
func (twokConfigurableParams) UpgradeIgnitionHeight() abi.ChainEpoch {
	return twokUpgradeIgnitionHeight
}
func (twokConfigurableParams) UpgradeRefuelHeight() abi.ChainEpoch {
	return twokUpgradeRefuelHeight
}
func (twokConfigurableParams) UpgradeActorsV2Height() abi.ChainEpoch {
	return twokUpgradeActorsV2Height
}
func (twokConfigurableParams) UpgradeTapeHeight() abi.ChainEpoch {
	return twokUpgradeTapeHeight
}
func (twokConfigurableParams) UpgradeLiftoffHeight() abi.ChainEpoch {
	return twokUpgradeLiftoffHeight
}
func (twokConfigurableParams) UpgradeKumquatHeight() abi.ChainEpoch {
	return twokUpgradeKumquatHeight
}
func (twokConfigurableParams) UpgradeCalicoHeight() abi.ChainEpoch {
	return twokUpgradeCalicoHeight
}
func (twokConfigurableParams) UpgradePersianHeight() abi.ChainEpoch {
	return twokUpgradePersianHeight
}
func (twokConfigurableParams) UpgradeOrangeHeight() abi.ChainEpoch {
	return twokUpgradeOrangeHeight
}
func (twokConfigurableParams) UpgradeClausHeight() abi.ChainEpoch {
	return twokUpgradeClausHeight
}
func (twokConfigurableParams) UpgradeActorsV3Height() abi.ChainEpoch {
	return twokUpgradeActorsV3Height
}
func (twokConfigurableParams) UpgradeNorwegianHeight() abi.ChainEpoch {
	return twokUpgradeNorwegianHeight
}
func (twokConfigurableParams) UpgradeActorsV4Height() abi.ChainEpoch {
	return twokUpgradeActorsV4Height
}
func (twokConfigurableParams) BlockDelaySecs() uint64 {
	return twokBlockDelaySecs
}
func (twokConfigurableParams) PropagationDelaySecs() uint64 {
	return twokPropagationDelaySecs
}
func (twokConfigurableParams) BootstrapPeerThreshold() int {
	return twokBootstrapPeerThreshold
}
func (twokConfigurableParams) WhitelistedBlock() cid.Cid {
	return twokWhitelistedBlock
}
func (twokConfigurableParams) InsecurePoStValidation() bool {
	return true
}
func (twokConfigurableParams) Devnet() bool {
	return true
}
