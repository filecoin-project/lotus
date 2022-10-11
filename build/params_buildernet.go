//go:build buildernet
// +build buildernet

package build

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/policy"
)

const Consensus ConsensusType = Mir

// FIXME: Using devnet actors and genesis for now.
const BootstrappersFile = ""
const GenesisFile = ""

var NetworkBundle = "devnet-wasm"
var BundleOverrides map[actors.Version]string
var ActorDebugging = false

// FIXME: The following will be used to address this issue:
// https://github.com/consensus-shipyard/lotus/issues/13
//
// var NetworkBundle = "wallaby"
// var BundleOverrides map[actors.Version]string
// var ActorDebugging = true

// const BootstrappersFile = "wallabynet.pi"
// const GenesisFile = "wallabynet.car"

const GenesisNetworkVersion = network.Version16

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

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg2KiBV1,
	abi.RegisteredSealProof_StackedDrg8MiBV1,
}

var ConsensusMinerMinPower = abi.NewStoragePower(2048)
var MinVerifiedDealSize = abi.NewStoragePower(256)
var PreCommitChallengeDelay = abi.ChainEpoch(10)

// FIXME: For now we are using debug storage and deal sizes.
// In the future uncomment this to support real size deals and sectors.
// and comment the lines above.
//
// func init() {
// 	policy.SetSupportedProofTypes(SupportedProofTypes...)
// 	policy.SetConsensusMinerMinPower(ConsensusMinerMinPower)
// 	policy.SetMinVerifiedDealSize(MinVerifiedDealSize)
// 	policy.SetPreCommitChallengeDelay(PreCommitChallengeDelay)

// var SupportedProofTypes = []abi.RegisteredSealProof{
// 	abi.RegisteredSealProof_StackedDrg512MiBV1,
// 	abi.RegisteredSealProof_StackedDrg32GiBV1,
// 	abi.RegisteredSealProof_StackedDrg64GiBV1,
// }
// var ConsensusMinerMinPower = abi.NewStoragePower(16 << 30)
// var MinVerifiedDealSize = abi.NewStoragePower(1 << 20)
// var PreCommitChallengeDelay = abi.ChainEpoch(10)

func init() {
	policy.SetSupportedProofTypes(SupportedProofTypes...)
	policy.SetConsensusMinerMinPower(ConsensusMinerMinPower)
	policy.SetMinVerifiedDealSize(MinVerifiedDealSize)
	policy.SetPreCommitChallengeDelay(PreCommitChallengeDelay)

	BuildType = BuildBuildernet
	SetAddressNetwork(address.Testnet)
	Devnet = true

}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const BootstrapPeerThreshold = 2

// Eip155ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = 31415

var WhitelistedBlock = cid.Undef
