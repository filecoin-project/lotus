//go:build butterflynet
// +build butterflynet

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

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

const GenesisNetworkVersion = network.Version17

var NetworkBundle = "butterflynet"
var BundleOverrides map[actorstypes.Version]string
var ActorDebugging = false

const BootstrappersFile = "butterflynet.pi"
const GenesisFile = "butterflynet.car"

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
const UpgradeSharkHeight = abi.ChainEpoch(-20)
const UpgradeHyggeHeight = abi.ChainEpoch(600)

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg512MiBV1,
	abi.RegisteredSealProof_StackedDrg32GiBV1,
	abi.RegisteredSealProof_StackedDrg64GiBV1,
}
var ConsensusMinerMinPower = abi.NewStoragePower(2 << 30)
var MinVerifiedDealSize = abi.NewStoragePower(1 << 20)
var PreCommitChallengeDelay = abi.ChainEpoch(150)

func init() {
	policy.SetSupportedProofTypes(SupportedProofTypes...)
	policy.SetConsensusMinerMinPower(ConsensusMinerMinPower)
	policy.SetMinVerifiedDealSize(MinVerifiedDealSize)
	policy.SetPreCommitChallengeDelay(PreCommitChallengeDelay)

	SetAddressNetwork(address.Testnet)

	Devnet = true

	BuildType = BuildButterflynet
}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const BootstrapPeerThreshold = 2

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = 3141592

var WhitelistedBlock = cid.Undef
