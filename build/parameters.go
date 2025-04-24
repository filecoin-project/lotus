package build

import (
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

var IsNearUpgrade = buildconstants.IsNearUpgrade // Deprecated: Use buildconstants.IsNearUpgrade instead

var GenesisNetworkVersion = buildconstants.GenesisNetworkVersion // Deprecated: Use buildconstants.GenesisNetworkVersion instead

var UpgradeBreezeHeight = buildconstants.UpgradeBreezeHeight // Deprecated: Use buildconstants.UpgradeBreezeHeight instead

var BreezeGasTampingDuration = buildconstants.BreezeGasTampingDuration // Deprecated: Use buildconstants.BreezeGasTampingDuration instead

// upgrade heights
var UpgradeSmokeHeight = buildconstants.UpgradeSmokeHeight           // Deprecated: Use buildconstants.UpgradeSmokeHeight instead
var UpgradeIgnitionHeight = buildconstants.UpgradeIgnitionHeight     // Deprecated: Use buildconstants.UpgradeIgnitionHeight instead
var UpgradeRefuelHeight = buildconstants.UpgradeRefuelHeight         // Deprecated: Use buildconstants.UpgradeRefuelHeight instead
var UpgradeTapeHeight = buildconstants.UpgradeTapeHeight             // Deprecated: Use buildconstants.UpgradeTapeHeight instead
var UpgradeAssemblyHeight = buildconstants.UpgradeAssemblyHeight     // Deprecated: Use buildconstants.UpgradeAssemblyHeight instead
var UpgradeLiftoffHeight = buildconstants.UpgradeLiftoffHeight       // Deprecated: Use buildconstants.UpgradeLiftoffHeight instead
var UpgradeKumquatHeight = buildconstants.UpgradeKumquatHeight       // Deprecated: Use buildconstants.UpgradeKumquatHeight instead
var UpgradeCalicoHeight = buildconstants.UpgradeCalicoHeight         // Deprecated: Use buildconstants.UpgradeCalicoHeight instead
var UpgradePersianHeight = buildconstants.UpgradePersianHeight       // Deprecated: Use buildconstants.UpgradePersianHeight instead
var UpgradeOrangeHeight = buildconstants.UpgradeOrangeHeight         // Deprecated: Use buildconstants.UpgradeOrangeHeight instead
var UpgradeClausHeight = buildconstants.UpgradeClausHeight           // Deprecated: Use buildconstants.UpgradeClausHeight instead
var UpgradeTrustHeight = buildconstants.UpgradeTrustHeight           // Deprecated: Use buildconstants.UpgradeTrustHeight instead
var UpgradeNorwegianHeight = buildconstants.UpgradeNorwegianHeight   // Deprecated: Use buildconstants.UpgradeNorwegianHeight instead
var UpgradeTurboHeight = buildconstants.UpgradeTurboHeight           // Deprecated: Use buildconstants.UpgradeTurboHeight instead
var UpgradeHyperdriveHeight = buildconstants.UpgradeHyperdriveHeight // Deprecated: Use buildconstants.UpgradeHyperdriveHeight instead
var UpgradeChocolateHeight = buildconstants.UpgradeChocolateHeight   // Deprecated: Use buildconstants.UpgradeChocolateHeight instead
var UpgradeOhSnapHeight = buildconstants.UpgradeOhSnapHeight         // Deprecated: Use buildconstants.UpgradeOhSnapHeight instead
var UpgradeSkyrHeight = buildconstants.UpgradeSkyrHeight             // Deprecated: Use buildconstants.UpgradeSkyrHeight instead
var UpgradeSharkHeight = buildconstants.UpgradeSharkHeight           // Deprecated: Use buildconstants.UpgradeSharkHeight instead
var UpgradeHyggeHeight = buildconstants.UpgradeHyggeHeight           // Deprecated: Use buildconstants.UpgradeHyggeHeight instead
var UpgradeLightningHeight = buildconstants.UpgradeLightningHeight   // Deprecated: Use buildconstants.UpgradeLightningHeight instead
var UpgradeThunderHeight = buildconstants.UpgradeThunderHeight       // Deprecated: Use buildconstants.UpgradeThunderHeight instead
var UpgradeWatermelonHeight = buildconstants.UpgradeWatermelonHeight // Deprecated: Use buildconstants.UpgradeWatermelonHeight instead
var UpgradeDragonHeight = buildconstants.UpgradeDragonHeight         // Deprecated: Use buildconstants.UpgradeDragonHeight instead
var UpgradePhoenixHeight = buildconstants.UpgradePhoenixHeight       // Deprecated: Use buildconstants.UpgradePhoenixHeight instead
var UpgradeWaffleHeight = buildconstants.UpgradeWaffleHeight         // Deprecated: Use buildconstants.UpgradeWaffleHeight instead

// This fix upgrade only ran on calibrationnet
var UpgradeWatermelonFixHeight = buildconstants.UpgradeWatermelonFixHeight // Deprecated: Use buildconstants.UpgradeWatermelonFixHeight instead

// This fix upgrade only ran on calibrationnet
var UpgradeWatermelonFix2Height = buildconstants.UpgradeWatermelonFix2Height // Deprecated: Use buildconstants.UpgradeWatermelonFix2Height instead

// This fix upgrade only ran on calibrationnet
var UpgradeCalibrationDragonFixHeight = buildconstants.UpgradeCalibrationDragonFixHeight // Deprecated: Use buildconstants.UpgradeCalibrationDragonFixHeight instead

var ConsensusMinerMinPower = buildconstants.ConsensusMinerMinPower   // Deprecated: Use buildconstants.ConsensusMinerMinPower instead
var PreCommitChallengeDelay = buildconstants.PreCommitChallengeDelay // Deprecated: Use buildconstants.PreCommitChallengeDelay instead

var BlockDelaySecs = buildconstants.BlockDelaySecs // Deprecated: Use buildconstants.BlockDelaySecs instead

var PropagationDelaySecs = buildconstants.PropagationDelaySecs // Deprecated: Use buildconstants.PropagationDelaySecs instead

const BootstrapPeerThreshold = buildconstants.BootstrapPeerThreshold // Deprecated: Use buildconstants.BootstrapPeerThreshold instead

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = buildconstants.Eip155ChainId // Deprecated: Use buildconstants.Eip155ChainId instead

var WhitelistedBlock = buildconstants.WhitelistedBlock // Deprecated: Use buildconstants.WhitelistedBlock instead

const Finality = policy.ChainFinality // Deprecated: Use policy.ChainFinality instead
