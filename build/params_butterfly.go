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

const GenesisNetworkVersion = network.Version16

var (
	NetworkBundle   = "butterflynet"
	BundleOverrides map[actorstypes.Version]string
)

const (
	BootstrappersFile = "butterflynet.pi"
	GenesisFile       = "butterflynet.car"
)

const (
	UpgradeBreezeHeight      = -1
	BreezeGasTampingDuration = 120
	UpgradeSmokeHeight       = -2
	UpgradeIgnitionHeight    = -3
	UpgradeRefuelHeight      = -4
)

var UpgradeAssemblyHeight = abi.ChainEpoch(-5)

const (
	UpgradeTapeHeight       = -6
	UpgradeLiftoffHeight    = -7
	UpgradeKumquatHeight    = -8
	UpgradeCalicoHeight     = -9
	UpgradePersianHeight    = -10
	UpgradeClausHeight      = -11
	UpgradeOrangeHeight     = -12
	UpgradeTrustHeight      = -13
	UpgradeNorwegianHeight  = -14
	UpgradeTurboHeight      = -15
	UpgradeHyperdriveHeight = -16
	UpgradeChocolateHeight  = -17
	UpgradeOhSnapHeight     = -18
	UpgradeSkyrHeight       = -19
	UpgradeSharkHeight      = abi.ChainEpoch(600)
)

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg512MiBV1,
	abi.RegisteredSealProof_StackedDrg32GiBV1,
	abi.RegisteredSealProof_StackedDrg64GiBV1,
}

var (
	ConsensusMinerMinPower  = abi.NewStoragePower(2 << 30)
	MinVerifiedDealSize     = abi.NewStoragePower(1 << 20)
	PreCommitChallengeDelay = abi.ChainEpoch(150)
)

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
