//go:build calibnet
// +build calibnet

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

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

const GenesisNetworkVersion = network.Version0

var NetworkBundle = "calibrationnet"
var BundleOverrides map[actorstypes.Version]string
var ActorDebugging = false

const BootstrappersFile = "calibnet.pi"
const GenesisFile = "calibnet.car"

const UpgradeBreezeHeight = -1
const BreezeGasTampingDuration = 120

const UpgradeSmokeHeight = -2

const UpgradeIgnitionHeight = -3
const UpgradeRefuelHeight = -4

var UpgradeAssemblyHeight = abi.ChainEpoch(30)

const UpgradeTapeHeight = 60

const UpgradeLiftoffHeight = -5

const UpgradeKumquatHeight = 90

const UpgradeCalicoHeight = 120
const UpgradePersianHeight = UpgradeCalicoHeight + (builtin2.EpochsInHour * 1)

const UpgradeClausHeight = 270

const UpgradeOrangeHeight = 300

const UpgradeTrustHeight = 330

const UpgradeNorwegianHeight = 360

const UpgradeTurboHeight = 390

const UpgradeHyperdriveHeight = 420

const UpgradeChocolateHeight = 450

const UpgradeOhSnapHeight = 480

const UpgradeSkyrHeight = 510

const UpgradeSharkHeight = 16800 // 6 days after genesis

// 2023-02-21T16:30:00Z
const UpgradeHyggeHeight = 322354

var SupportedProofTypes = []abi.RegisteredSealProof{
	abi.RegisteredSealProof_StackedDrg32GiBV1,
	abi.RegisteredSealProof_StackedDrg64GiBV1,
}
var ConsensusMinerMinPower = abi.NewStoragePower(32 << 30)
var MinVerifiedDealSize = abi.NewStoragePower(1 << 20)
var PreCommitChallengeDelay = abi.ChainEpoch(150)

func init() {
	policy.SetSupportedProofTypes(SupportedProofTypes...)
	policy.SetConsensusMinerMinPower(ConsensusMinerMinPower)
	policy.SetMinVerifiedDealSize(MinVerifiedDealSize)
	policy.SetPreCommitChallengeDelay(PreCommitChallengeDelay)

	SetAddressNetwork(address.Testnet)

	Devnet = true

	// NOTE: DO NOT change this unless you REALLY know what you're doing. This is not consensus critical, however,
	//set this value too high may impacts your block submission; set this value too low may cause you miss
	//parent tipsets for blocking forming and mining.
	if len(os.Getenv("PROPAGATION_DELAY_SECS")) != 0 {
		pds, err := strconv.ParseUint(os.Getenv("PROPAGATION_DELAY_SECS"), 10, 64)
		if err != nil {
			log.Warnw("Error setting PROPAGATION_DELAY_SECS, %v, proceed with default value %s", err,
				PropagationDelaySecs)
		} else {
			PropagationDelaySecs = pds
			log.Warnw(" !!WARNING!! propagation delay is set to be %s second, "+
				"this value impacts your message republish interval and block forming - monitor with caution!!", PropagationDelaySecs)
		}
	}

	BuildType = BuildCalibnet

}

const BlockDelaySecs = uint64(builtin2.EpochDurationSeconds)

var PropagationDelaySecs = uint64(10)

// BootstrapPeerThreshold is the minimum number peers we need to track for a sync worker to start
const BootstrapPeerThreshold = 4

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = 314159

var WhitelistedBlock = cid.Undef
