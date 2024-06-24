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

var BootstrappersFile = buildconstants.BootstrappersFile // Deprecated: Use buildconstants.BootstrappersFile instead

var GenesisFile = buildconstants.GenesisFile // Deprecated: Use buildconstants.GenesisFile instead

var NetworkBundle = buildconstants.NetworkBundle   // Deprecated: Use buildconstants.NetworkBundle instead
var ActorDebugging = buildconstants.ActorDebugging // Deprecated: Use buildconstants.ActorDebugging instead

var GenesisNetworkVersion = buildconstants.GenesisNetworkVersion // Deprecated: Use buildconstants.GenesisNetworkVersion instead

var UpgradeBreezeHeight abi.ChainEpoch = buildconstants.UpgradeBreezeHeight // Deprecated: Use buildconstants.UpgradeBreezeHeight instead

var BreezeGasTampingDuration abi.ChainEpoch = buildconstants.BreezeGasTampingDuration // Deprecated: Use buildconstants.BreezeGasTampingDuration instead

// upgrade heights
var UpgradeSmokeHeight abi.ChainEpoch = buildconstants.UpgradeSmokeHeight           // Deprecated: Use buildconstants.UpgradeSmokeHeight instead
var UpgradeIgnitionHeight abi.ChainEpoch = buildconstants.UpgradeIgnitionHeight     // Deprecated: Use buildconstants.UpgradeIgnitionHeight instead
var UpgradeRefuelHeight abi.ChainEpoch = buildconstants.UpgradeRefuelHeight         // Deprecated: Use buildconstants.UpgradeRefuelHeight instead
var UpgradeTapeHeight abi.ChainEpoch = buildconstants.UpgradeTapeHeight             // Deprecated: Use buildconstants.UpgradeTapeHeight instead
var UpgradeAssemblyHeight abi.ChainEpoch = buildconstants.UpgradeAssemblyHeight     // Deprecated: Use buildconstants.UpgradeAssemblyHeight instead
var UpgradeLiftoffHeight abi.ChainEpoch = buildconstants.UpgradeLiftoffHeight       // Deprecated: Use buildconstants.UpgradeLiftoffHeight instead
var UpgradeKumquatHeight abi.ChainEpoch = buildconstants.UpgradeKumquatHeight       // Deprecated: Use buildconstants.UpgradeKumquatHeight instead
var UpgradeCalicoHeight abi.ChainEpoch = buildconstants.UpgradeCalicoHeight         // Deprecated: Use buildconstants.UpgradeCalicoHeight instead
var UpgradePersianHeight abi.ChainEpoch = buildconstants.UpgradePersianHeight       // Deprecated: Use buildconstants.UpgradePersianHeight instead
var UpgradeOrangeHeight abi.ChainEpoch = buildconstants.UpgradeOrangeHeight         // Deprecated: Use buildconstants.UpgradeOrangeHeight instead
var UpgradeClausHeight abi.ChainEpoch = buildconstants.UpgradeClausHeight           // Deprecated: Use buildconstants.UpgradeClausHeight instead
var UpgradeTrustHeight abi.ChainEpoch = buildconstants.UpgradeTrustHeight           // Deprecated: Use buildconstants.UpgradeTrustHeight instead
var UpgradeNorwegianHeight abi.ChainEpoch = buildconstants.UpgradeNorwegianHeight   // Deprecated: Use buildconstants.UpgradeNorwegianHeight instead
var UpgradeTurboHeight abi.ChainEpoch = buildconstants.UpgradeTurboHeight           // Deprecated: Use buildconstants.UpgradeTurboHeight instead
var UpgradeHyperdriveHeight abi.ChainEpoch = buildconstants.UpgradeHyperdriveHeight // Deprecated: Use buildconstants.UpgradeHyperdriveHeight instead
var UpgradeChocolateHeight abi.ChainEpoch = buildconstants.UpgradeChocolateHeight   // Deprecated: Use buildconstants.UpgradeChocolateHeight instead
var UpgradeOhSnapHeight abi.ChainEpoch = buildconstants.UpgradeOhSnapHeight         // Deprecated: Use buildconstants.UpgradeOhSnapHeight instead
var UpgradeSkyrHeight abi.ChainEpoch = buildconstants.UpgradeSkyrHeight             // Deprecated: Use buildconstants.UpgradeSkyrHeight instead
var UpgradeSharkHeight abi.ChainEpoch = buildconstants.UpgradeSharkHeight           // Deprecated: Use buildconstants.UpgradeSharkHeight instead
var UpgradeHyggeHeight abi.ChainEpoch = buildconstants.UpgradeHyggeHeight           // Deprecated: Use buildconstants.UpgradeHyggeHeight instead
var UpgradeLightningHeight abi.ChainEpoch = buildconstants.UpgradeLightningHeight   // Deprecated: Use buildconstants.UpgradeLightningHeight instead
var UpgradeThunderHeight abi.ChainEpoch = buildconstants.UpgradeThunderHeight       // Deprecated: Use buildconstants.UpgradeThunderHeight instead
var UpgradeWatermelonHeight abi.ChainEpoch = buildconstants.UpgradeWatermelonHeight // Deprecated: Use buildconstants.UpgradeWatermelonHeight instead
var UpgradeDragonHeight abi.ChainEpoch = buildconstants.UpgradeDragonHeight         // Deprecated: Use buildconstants.UpgradeDragonHeight instead
var UpgradePhoenixHeight abi.ChainEpoch = buildconstants.UpgradePhoenixHeight       // Deprecated: Use buildconstants.UpgradePhoenixHeight instead
var UpgradeWaffleHeight abi.ChainEpoch = buildconstants.UpgradeWaffleHeight         // Deprecated: Use buildconstants.UpgradeWaffleHeight instead

// This fix upgrade only ran on calibrationnet
var UpgradeWatermelonFixHeight abi.ChainEpoch = buildconstants.UpgradeWatermelonFixHeight // Deprecated: Use buildconstants.UpgradeWatermelonFixHeight instead

// This fix upgrade only ran on calibrationnet
var UpgradeWatermelonFix2Height abi.ChainEpoch = buildconstants.UpgradeWatermelonFix2Height // Deprecated: Use buildconstants.UpgradeWatermelonFix2Height instead

// This fix upgrade only ran on calibrationnet
var UpgradeCalibrationDragonFixHeight abi.ChainEpoch = buildconstants.UpgradeCalibrationDragonFixHeight // Deprecated: Use buildconstants.UpgradeCalibrationDragonFixHeight instead

var SupportedProofTypes = buildconstants.SupportedProofTypes         // Deprecated: Use buildconstants.SupportedProofTypes instead
var ConsensusMinerMinPower = buildconstants.ConsensusMinerMinPower   // Deprecated: Use buildconstants.ConsensusMinerMinPower instead
var PreCommitChallengeDelay = buildconstants.PreCommitChallengeDelay // Deprecated: Use buildconstants.PreCommitChallengeDelay instead

var BlockDelaySecs = buildconstants.BlockDelaySecs // Deprecated: Use buildconstants.BlockDelaySecs instead

var PropagationDelaySecs = buildconstants.PropagationDelaySecs // Deprecated: Use buildconstants.PropagationDelaySecs instead

var EquivocationDelaySecs = buildconstants.EquivocationDelaySecs // Deprecated: Use buildconstants.EquivocationDelaySecs instead

const BootstrapPeerThreshold = buildconstants.BootstrapPeerThreshold // Deprecated: Use buildconstants.BootstrapPeerThreshold instead

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = buildconstants.Eip155ChainId // Deprecated: Use buildconstants.Eip155ChainId instead

var WhitelistedBlock = buildconstants.WhitelistedBlock // Deprecated: Use buildconstants.WhitelistedBlock instead

const Finality = policy.ChainFinality // Deprecated: Use policy.ChainFinality instead
