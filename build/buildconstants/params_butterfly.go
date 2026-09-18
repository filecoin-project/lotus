//go:build butterflynet

package buildconstants

import (
	_ "embed"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandQuicknet,
}

const GenesisNetworkVersion = network.Version28

var NetworkBundle = "butterflynet"
var ActorDebugging = false

const BootstrappersFile = "butterflynet.pi"
const GenesisFile = "butterflynet.car.zst"

const UpgradeBreezeHeight = -1
const BreezeGasTampingDuration = 120
const UpgradeSmokeHeight = -2
const UpgradeIgnitionHeight = -3
const UpgradeRefuelHeight = -4

var UpgradeAssemblyHeight = abi.ChainEpoch(-5)

const UpgradeTapeHeight = -6
const UpgradeLiftoffHeight = -7
const UpgradeKumquatHeight = -8
const UpgradeCalicoHeight = -9
const UpgradePersianHeight = -10
const UpgradeClausHeight = -11
const UpgradeOrangeHeight = -12
const UpgradeTrustHeight = -13
const UpgradeNorwegianHeight = -14
const UpgradeTurboHeight = -15
const UpgradeHyperdriveHeight = -16
const UpgradeChocolateHeight = -17
const UpgradeOhSnapHeight = -18
const UpgradeSkyrHeight = -19
const UpgradeSharkHeight = -20
const UpgradeHyggeHeight = -21
const UpgradeLightningHeight = -22
const UpgradeThunderHeight = -23
const UpgradeWatermelonHeight = -24

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFixHeight = -100

// This fix upgrade only ran on calibrationnet
const UpgradeWatermelonFix2Height = -101
const UpgradeDragonHeight = -25

// This fix upgrade only ran on calibrationnet
const UpgradeCalibrationDragonFixHeight = -102
const UpgradePhoenixHeight = -26
const UpgradeWaffleHeight = -27
const UpgradeTuktukHeight = -28

// FIP-0081: for the power actor state for pledge calculations.
// UpgradeTuktukPowerRampDurationEpochs ends up in the power actor state after
// Tuktuk migration. along with a RampStartEpoch matching the upgrade height.
var UpgradeTuktukPowerRampDurationEpochs = uint64(builtin.EpochsInYear)

const UpgradeTeepHeight = -29

var UpgradeTeepInitialFilReserved = wholeFIL(1_600_000_000) // FIP-0100: 300M -> 1.6B FIL

const UpgradeTockHeight = -30

// This fix upgrade only ran on calibrationnet
const UpgradeTockFixHeight = -103

var UpgradeGoldenWeekHeight = abi.ChainEpoch(-31)

const UpgradeFireHorseHeight = -32

const UpgradeSolsticeHeight = 6060

// SolsticeEpochsPerQuarter matches the quarter the SRA is deployed with: two hours on
// butterflynet. The ramp runs nine of them.
const SolsticeEpochsPerQuarter = abi.ChainEpoch(builtin2.EpochsInHour * 2)

// FIP-0118: reward actor bootstrap state installed by the Solstice migration.
//
// script/Deploy.s.sol in the solstice repo deploys the contracts from a throwaway key, published
// here so anyone can redeploy after a butterflynet reset:
//
//	key 1, deployer, owner 1 and InitialOrchestrator
//	  0x50891ab8b7707035ff59b3738aa36e8ddd6c996bd40799f8a41ce166ef0c99ea
//	  0x48C7DC38e74C9fA9eA6484Ad6Ad0520349dC9B40
//	key 2, owner 2
//	  0x09a31df81dc2d88091221894caf8fc43f6b9e59e88f82383b500fc244c936ed8
//	  0x831246Ec4A91eF36acA821068b95A9EF1765C514
//
// Deployer nonce 0 is the SRA implementation, 1 the SRA proxy, 2 the SWA implementation, 3 the
// SWA proxy, so the two proxy addresses below follow from the deployer address alone. Fund key 1,
// then take all four nonces in one run from the solstice repo, against the Eth RPC so the sender
// is the f410 form of the deployer:
//
//	forge script script/Deploy.s.sol --broadcast --skip-simulation \
//	  --rpc-url $ETH_RPC_URL --private-key 0x5089...99ea
var UpgradeSolsticeRewardBootstrapParams = SolsticeRewardBootstrapParams{
	SWATimelockEpochs:                 40, // twenty minutes
	ConsensusWeightRampDurationEpochs: SolsticeEpochsPerQuarter * 9,
	ConsensusWeight: SolsticeRewardWeightParams{
		VStart: 95 * solsticeRewardWeightPercent,
		Floor:  50 * solsticeRewardWeightPercent,
		Cap:    95 * solsticeRewardWeightPercent,
	},
	ServiceWeight: SolsticeRewardWeightParams{
		VStart: 5 * solsticeRewardWeightPercent,
		Floor:  5 * solsticeRewardWeightPercent,
		Cap:    10 * solsticeRewardWeightPercent,
	},
	SWAActor:            MustParseFilOrEthAddress("0x17c43bC9d8E8600ebE7599C18f2dA2D5CED68D95"),
	SRAActor:            MustParseFilOrEthAddress("0xea340224F4df7D01d2657964215E37452165b0A1"),
	InitialOrchestrator: MustParseFilOrEthAddress("0x48C7DC38e74C9fA9eA6484Ad6Ad0520349dC9B40"),
}

var ConsensusMinerMinPower = abi.NewStoragePower(2 << 30)
var PreCommitChallengeDelay = abi.ChainEpoch(150)

func init() {
	SetAddressNetwork(address.Testnet)

	Devnet = true

	BuildType = BuildButterflynet
}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)

var EquivocationDelaySecs = uint64(2)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const BootstrapPeerThreshold = 2

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = 3141592

var WhitelistedBlock = cid.Undef

const F3Enabled = true

//go:embed f3manifest_butterfly.json
var F3ManifestBytes []byte
