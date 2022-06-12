//go:build butterflynet
// +build butterflynet

package build

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	"github.com/ipfs/go-cid"
)

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

const GenesisNetworkVersion = network.Version14

var NetworkBundle = "butterflynet"

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

const UpgradeOhSnapHeight = 240

// 2022-05-31T14:32:00Z
const UpgradeSkyrHeight = abi.ChainEpoch(333258)

var ActorsCIDs = map[actors.Version]cid.Cid{
	actors.Version8: MustParseCid("bafy2bzacedy4qgxbr6pbyfgcp7s7bdkc2whi5errnw67al5e2tk75j46iucv6"),
}

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

var WhitelistedBlock = cid.Undef
