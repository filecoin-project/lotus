//go:build hyperspacenet
// +build hyperspacenet

package build

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/network"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

var NetworkBundle = "hyperspace"
var BundleOverrides map[actorstypes.Version]string
var ActorDebugging = false

const BootstrappersFile = "hyperspacenet.pi"
const GenesisFile = "hyperspacenet.car"

const GenesisNetworkVersion = network.Version18

var UpgradeBreezeHeight = abi.ChainEpoch(-1)

const BreezeGasTampingDuration = 120

var UpgradeSmokeHeight = abi.ChainEpoch(-1)
var UpgradeIgnitionHeight = abi.ChainEpoch(-2)
var UpgradeRefuelHeight = abi.ChainEpoch(-3)
var UpgradeTapeHeight = abi.ChainEpoch(-4)

var UpgradeAssemblyHeight = abi.ChainEpoch(-5)
var UpgradeLiftoffHeight = abi.ChainEpoch(-6)

var UpgradeKumquatHeight = abi.ChainEpoch(-7)
var UpgradeCalicoHeight = abi.ChainEpoch(-9)
var UpgradePersianHeight = abi.ChainEpoch(-10)
var UpgradeOrangeHeight = abi.ChainEpoch(-11)
var UpgradeClausHeight = abi.ChainEpoch(-12)

var UpgradeTrustHeight = abi.ChainEpoch(-13)

var UpgradeNorwegianHeight = abi.ChainEpoch(-14)

var UpgradeTurboHeight = abi.ChainEpoch(-15)

var UpgradeHyperdriveHeight = abi.ChainEpoch(-16)
var UpgradeChocolateHeight = abi.ChainEpoch(-17)
var UpgradeOhSnapHeight = abi.ChainEpoch(-18)
var UpgradeSkyrHeight = abi.ChainEpoch(-19)
var UpgradeSharkHeight = abi.ChainEpoch(-20)
var UpgradeHyggeHeight = abi.ChainEpoch(-21)

// 2023-01-18T15:00:00Z
var UpgradeHyperspaceNV19Height = abi.ChainEpoch(6840)

// 2023-02-15T15:00:00Z
var UpgradeHyperspaceNV20Height = abi.ChainEpoch(87480)

// 2023-02-27T15:00:00Z
var UpgradeHyperspaceNV21Height = abi.ChainEpoch(122040)

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg512MiBV1,
	abi.RegisteredSealProof_StackedDrg32GiBV1,
	abi.RegisteredSealProof_StackedDrg64GiBV1,
}
var ConsensusMinerMinPower = abi.NewStoragePower(16 << 30)
var MinVerifiedDealSize = abi.NewStoragePower(1 << 20)
var PreCommitChallengeDelay = abi.ChainEpoch(10)

func init() {
	policy.SetSupportedProofTypes(SupportedProofTypes...)
	policy.SetConsensusMinerMinPower(ConsensusMinerMinPower)
	policy.SetMinVerifiedDealSize(MinVerifiedDealSize)
	policy.SetPreCommitChallengeDelay(PreCommitChallengeDelay)

	BuildType = BuildHyperspacenet
	SetAddressNetwork(address.Testnet)
	Devnet = true

}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const BootstrapPeerThreshold = 2

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = 3141

var WhitelistedBlock = cid.Undef
