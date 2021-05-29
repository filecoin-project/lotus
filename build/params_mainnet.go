package build

import (
	"github.com/filecoin-project/go-state-types/abi"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	"github.com/ipfs/go-cid"
)

var mainDrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0:                      DrandIncentinet,
	mainUpgradeSmokeHeight: DrandMainnet,
}

const mainBootstrappersFile = "mainnet.pi"
const mainGenesisFile = "mainnet.car"

const mainUpgradeBreezeHeight = 41280

const mainBreezeGasTampingDuration = 120

const mainUpgradeSmokeHeight = 51000

const mainUpgradeIgnitionHeight = 94000
const mainUpgradeRefuelHeight = 130800

const mainUpgradeActorsV2Height = 138720

const mainUpgradeTapeHeight = 140760

// This signals our tentative epoch for mainnet launch. Can make it later, but not earlier.
// Miners, clients, developers, custodians all need time to prepare.
// We still have upgrades and state changes to do, but can happen after signaling timing here.
const mainUpgradeLiftoffHeight = 148888

const mainUpgradeKumquatHeight = 170000

const mainUpgradeCalicoHeight = 265200
const mainUpgradePersianHeight = mainUpgradeCalicoHeight + (builtin2.EpochsInHour * 60)

const mainUpgradeOrangeHeight = 336458

// 2020-12-22T02:00:00Z
const mainUpgradeClausHeight = 343200

// 2021-03-04T00:00:30Z
const mainUpgradeActorsV3Height = 550321

// 2021-04-12T22:00:00Z
const mainUpgradeNorwegianHeight = 665280

// 2021-04-29T06:00:00Z
const mainUpgradeActorsV4Height = 712320

const mainBlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const mainPropagationDelaySecs = uint64(6)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const mainBootstrapPeerThreshold = 4

// we skip checks on message validity in this block to sidestep the zero-bls signature
var mainWhitelistedBlock = MustParseCid("bafy2bzaceapyg2uyzk7vueh3xccxkuwbz3nxewjyguoxvhx77malc2lzn2ybi")

type mainConfigurableParams struct{}

func (mainConfigurableParams) DrandSchedule() map[abi.ChainEpoch]DrandEnum {
	return mainDrandSchedule
}
func (mainConfigurableParams) BootstrappersFile() string {
	return mainBootstrappersFile
}
func (mainConfigurableParams) GenesisFile() string {
	return mainGenesisFile
}
func (mainConfigurableParams) UpgradeBreezeHeight() abi.ChainEpoch {
	return mainUpgradeBreezeHeight
}
func (mainConfigurableParams) BreezeGasTampingDuration() abi.ChainEpoch {
	return mainBreezeGasTampingDuration
}
func (mainConfigurableParams) UpgradeSmokeHeight() abi.ChainEpoch {
	return mainUpgradeSmokeHeight
}
func (mainConfigurableParams) UpgradeIgnitionHeight() abi.ChainEpoch {
	return mainUpgradeIgnitionHeight
}
func (mainConfigurableParams) UpgradeRefuelHeight() abi.ChainEpoch {
	return mainUpgradeRefuelHeight
}
func (mainConfigurableParams) UpgradeActorsV2Height() abi.ChainEpoch {
	return mainUpgradeActorsV2Height
}
func (mainConfigurableParams) UpgradeTapeHeight() abi.ChainEpoch {
	return mainUpgradeTapeHeight
}
func (mainConfigurableParams) UpgradeLiftoffHeight() abi.ChainEpoch {
	return mainUpgradeLiftoffHeight
}
func (mainConfigurableParams) UpgradeKumquatHeight() abi.ChainEpoch {
	return mainUpgradeKumquatHeight
}
func (mainConfigurableParams) UpgradeCalicoHeight() abi.ChainEpoch {
	return mainUpgradeCalicoHeight
}
func (mainConfigurableParams) UpgradePersianHeight() abi.ChainEpoch {
	return mainUpgradePersianHeight
}
func (mainConfigurableParams) UpgradeOrangeHeight() abi.ChainEpoch {
	return mainUpgradeOrangeHeight
}
func (mainConfigurableParams) UpgradeClausHeight() abi.ChainEpoch {
	return mainUpgradeClausHeight
}
func (mainConfigurableParams) UpgradeActorsV3Height() abi.ChainEpoch {
	return mainUpgradeActorsV3Height
}
func (mainConfigurableParams) UpgradeNorwegianHeight() abi.ChainEpoch {
	return mainUpgradeNorwegianHeight
}
func (mainConfigurableParams) UpgradeActorsV4Height() abi.ChainEpoch {
	return mainUpgradeActorsV4Height
}
func (mainConfigurableParams) BlockDelaySecs() uint64 {
	return mainBlockDelaySecs
}
func (mainConfigurableParams) PropagationDelaySecs() uint64 {
	return mainPropagationDelaySecs
}
func (mainConfigurableParams) BootstrapPeerThreshold() int {
	return mainBootstrapPeerThreshold
}
func (mainConfigurableParams) WhitelistedBlock() cid.Cid {
	return mainWhitelistedBlock
}
func (mainConfigurableParams) InsecurePoStValidation() bool {
	return false
}
func (mainConfigurableParams) Devnet() bool {
	return false
}
