package build

import (
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"

	"github.com/filecoin-project/lotus/build/buildconstants"
	proofparams "github.com/filecoin-project/lotus/build/proof-params"
	"github.com/filecoin-project/lotus/chain/actors/policy"
)

var ParametersJSON = proofparams.ParametersJSON
var SrsJSON = proofparams.SrsJSON

// NOTE: DO NOT change this unless you REALLY know what you're doing. This is consensus critical.
var BundleOverrides map[actorstypes.Version]string

var BootstrappersFile = buildconstants.BootstrappersFile

var GenesisFile = buildconstants.GenesisFile

var NetworkBundle = buildconstants.NetworkBundle
var ActorDebugging = buildconstants.ActorDebugging

var GenesisNetworkVersion = buildconstants.GenesisNetworkVersion

var UpgradeBreezeHeight abi.ChainEpoch = buildconstants.UpgradeBreezeHeight

var BreezeGasTampingDuration abi.ChainEpoch = buildconstants.BreezeGasTampingDuration

// upgrade heights
var UpgradeSmokeHeight abi.ChainEpoch = buildconstants.UpgradeSmokeHeight
var UpgradeIgnitionHeight abi.ChainEpoch = buildconstants.UpgradeIgnitionHeight
var UpgradeRefuelHeight abi.ChainEpoch = buildconstants.UpgradeRefuelHeight
var UpgradeTapeHeight abi.ChainEpoch = buildconstants.UpgradeTapeHeight
var UpgradeAssemblyHeight abi.ChainEpoch = buildconstants.UpgradeAssemblyHeight
var UpgradeLiftoffHeight abi.ChainEpoch = buildconstants.UpgradeLiftoffHeight
var UpgradeKumquatHeight abi.ChainEpoch = buildconstants.UpgradeKumquatHeight
var UpgradeCalicoHeight abi.ChainEpoch = buildconstants.UpgradeCalicoHeight
var UpgradePersianHeight abi.ChainEpoch = buildconstants.UpgradePersianHeight
var UpgradeOrangeHeight abi.ChainEpoch = buildconstants.UpgradeOrangeHeight
var UpgradeClausHeight abi.ChainEpoch = buildconstants.UpgradeClausHeight
var UpgradeTrustHeight abi.ChainEpoch = buildconstants.UpgradeTrustHeight
var UpgradeNorwegianHeight abi.ChainEpoch = buildconstants.UpgradeNorwegianHeight
var UpgradeTurboHeight abi.ChainEpoch = buildconstants.UpgradeTurboHeight
var UpgradeHyperdriveHeight abi.ChainEpoch = buildconstants.UpgradeHyperdriveHeight
var UpgradeChocolateHeight abi.ChainEpoch = buildconstants.UpgradeChocolateHeight
var UpgradeOhSnapHeight abi.ChainEpoch = buildconstants.UpgradeOhSnapHeight
var UpgradeSkyrHeight abi.ChainEpoch = buildconstants.UpgradeSkyrHeight
var UpgradeSharkHeight abi.ChainEpoch = buildconstants.UpgradeSharkHeight
var UpgradeHyggeHeight abi.ChainEpoch = buildconstants.UpgradeHyggeHeight
var UpgradeLightningHeight abi.ChainEpoch = buildconstants.UpgradeLightningHeight
var UpgradeThunderHeight abi.ChainEpoch = buildconstants.UpgradeThunderHeight
var UpgradeWatermelonHeight abi.ChainEpoch = buildconstants.UpgradeWatermelonHeight
var UpgradeDragonHeight abi.ChainEpoch = buildconstants.UpgradeDragonHeight
var UpgradePhoenixHeight abi.ChainEpoch = buildconstants.UpgradePhoenixHeight
var UpgradeAussieHeight abi.ChainEpoch = buildconstants.UpgradeAussieHeight

// This fix upgrade only ran on calibrationnet
var UpgradeWatermelonFixHeight abi.ChainEpoch = buildconstants.UpgradeWatermelonFixHeight

// This fix upgrade only ran on calibrationnet
var UpgradeWatermelonFix2Height abi.ChainEpoch = buildconstants.UpgradeWatermelonFix2Height

// This fix upgrade only ran on calibrationnet
var UpgradeCalibrationDragonFixHeight abi.ChainEpoch = buildconstants.UpgradeCalibrationDragonFixHeight

var SupportedProofTypes = buildconstants.SupportedProofTypes
var ConsensusMinerMinPower = buildconstants.ConsensusMinerMinPower
var PreCommitChallengeDelay = buildconstants.PreCommitChallengeDelay

var BlockDelaySecs = buildconstants.BlockDelaySecs

var PropagationDelaySecs = buildconstants.PropagationDelaySecs

var EquivocationDelaySecs = buildconstants.EquivocationDelaySecs

const BootstrapPeerThreshold = buildconstants.BootstrapPeerThreshold

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = buildconstants.Eip155ChainId

var WhitelistedBlock = buildconstants.WhitelistedBlock

const Finality = policy.ChainFinality
