//go:build interopnet
// +build interopnet

package build

import (
	"os"
	"strconv"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/network"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

var (
	NetworkBundle   = "caterpillarnet"
	BundleOverrides map[actorstypes.Version]string
	ActorDebugging  = false
)

const (
	BootstrappersFile = "interopnet.pi"
	GenesisFile       = "interopnet.car"
)

const GenesisNetworkVersion = network.Version16

var UpgradeBreezeHeight = abi.ChainEpoch(-1)

const BreezeGasTampingDuration = 0

var (
	UpgradeSmokeHeight      = abi.ChainEpoch(-1)
	UpgradeIgnitionHeight   = abi.ChainEpoch(-2)
	UpgradeRefuelHeight     = abi.ChainEpoch(-3)
	UpgradeTapeHeight       = abi.ChainEpoch(-4)
	UpgradeAssemblyHeight   = abi.ChainEpoch(-5)
	UpgradeLiftoffHeight    = abi.ChainEpoch(-6)
	UpgradeKumquatHeight    = abi.ChainEpoch(-7)
	UpgradeCalicoHeight     = abi.ChainEpoch(-9)
	UpgradePersianHeight    = abi.ChainEpoch(-10)
	UpgradeOrangeHeight     = abi.ChainEpoch(-11)
	UpgradeClausHeight      = abi.ChainEpoch(-12)
	UpgradeTrustHeight      = abi.ChainEpoch(-13)
	UpgradeNorwegianHeight  = abi.ChainEpoch(-14)
	UpgradeTurboHeight      = abi.ChainEpoch(-15)
	UpgradeHyperdriveHeight = abi.ChainEpoch(-16)
	UpgradeChocolateHeight  = abi.ChainEpoch(-17)
	UpgradeOhSnapHeight     = abi.ChainEpoch(-18)
	UpgradeSkyrHeight       = abi.ChainEpoch(-19)
	UpgradeSharkHeight      = abi.ChainEpoch(-20)
	UpgradeHyggeHeight      = abi.ChainEpoch(-21)
	UpgradeLightningHeight  = abi.ChainEpoch(-22)
)

const UpgradeThunderHeight = 50

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg2KiBV1,
	abi.RegisteredSealProof_StackedDrg8MiBV1,
	abi.RegisteredSealProof_StackedDrg512MiBV1,
}

var (
	ConsensusMinerMinPower  = abi.NewStoragePower(2048)
	MinVerifiedDealSize     = abi.NewStoragePower(256)
	PreCommitChallengeDelay = abi.ChainEpoch(10)
)

func init() {
	policy.SetSupportedProofTypes(SupportedProofTypes...)
	policy.SetConsensusMinerMinPower(ConsensusMinerMinPower)
	policy.SetMinVerifiedDealSize(MinVerifiedDealSize)
	policy.SetPreCommitChallengeDelay(PreCommitChallengeDelay)

	getUpgradeHeight := func(ev string, def abi.ChainEpoch) abi.ChainEpoch {
		hs, found := os.LookupEnv(ev)
		if found {
			h, err := strconv.Atoi(hs)
			if err != nil {
				log.Panicf("failed to parse %s env var", ev)
			}

			return abi.ChainEpoch(h)
		}

		return def
	}

	UpgradeBreezeHeight = getUpgradeHeight("LOTUS_BREEZE_HEIGHT", UpgradeBreezeHeight)
	UpgradeSmokeHeight = getUpgradeHeight("LOTUS_SMOKE_HEIGHT", UpgradeSmokeHeight)
	UpgradeIgnitionHeight = getUpgradeHeight("LOTUS_IGNITION_HEIGHT", UpgradeIgnitionHeight)
	UpgradeRefuelHeight = getUpgradeHeight("LOTUS_REFUEL_HEIGHT", UpgradeRefuelHeight)
	UpgradeTapeHeight = getUpgradeHeight("LOTUS_TAPE_HEIGHT", UpgradeTapeHeight)
	UpgradeAssemblyHeight = getUpgradeHeight("LOTUS_ACTORSV2_HEIGHT", UpgradeAssemblyHeight)
	UpgradeLiftoffHeight = getUpgradeHeight("LOTUS_LIFTOFF_HEIGHT", UpgradeLiftoffHeight)
	UpgradeKumquatHeight = getUpgradeHeight("LOTUS_KUMQUAT_HEIGHT", UpgradeKumquatHeight)
	UpgradeCalicoHeight = getUpgradeHeight("LOTUS_CALICO_HEIGHT", UpgradeCalicoHeight)
	UpgradePersianHeight = getUpgradeHeight("LOTUS_PERSIAN_HEIGHT", UpgradePersianHeight)
	UpgradeOrangeHeight = getUpgradeHeight("LOTUS_ORANGE_HEIGHT", UpgradeOrangeHeight)
	UpgradeClausHeight = getUpgradeHeight("LOTUS_CLAUS_HEIGHT", UpgradeClausHeight)
	UpgradeTrustHeight = getUpgradeHeight("LOTUS_ACTORSV3_HEIGHT", UpgradeTrustHeight)
	UpgradeNorwegianHeight = getUpgradeHeight("LOTUS_NORWEGIAN_HEIGHT", UpgradeNorwegianHeight)
	UpgradeTurboHeight = getUpgradeHeight("LOTUS_ACTORSV4_HEIGHT", UpgradeTurboHeight)
	UpgradeHyperdriveHeight = getUpgradeHeight("LOTUS_HYPERDRIVE_HEIGHT", UpgradeHyperdriveHeight)
	UpgradeChocolateHeight = getUpgradeHeight("LOTUS_CHOCOLATE_HEIGHT", UpgradeChocolateHeight)
	UpgradeOhSnapHeight = getUpgradeHeight("LOTUS_OHSNAP_HEIGHT", UpgradeOhSnapHeight)
	UpgradeSkyrHeight = getUpgradeHeight("LOTUS_SKYR_HEIGHT", UpgradeSkyrHeight)
	UpgradeSharkHeight = getUpgradeHeight("LOTUS_SHARK_HEIGHT", UpgradeSharkHeight)
	UpgradeHyggeHeight = getUpgradeHeight("LOTUS_HYGGE_HEIGHT", UpgradeHyggeHeight)

	BuildType |= BuildInteropnet
	SetAddressNetwork(address.Testnet)
	Devnet = true
}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start.
const BootstrapPeerThreshold = 2

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
// TODO same as butterfly for now, as we didn't submit an assignment for interopnet.
const Eip155ChainId = 3141592

var WhitelistedBlock = cid.Undef
