//go:build testground
// +build testground

// This file makes hardcoded parameters (const) configurable as vars.
//
// Its purpose is to unlock various degrees of flexibility and parametrization
// when writing Testground plans for Lotus.
package build

import (
	"math/big"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/network"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

var (
	UnixfsChunkSize     = uint64(1 << 20)
	UnixfsLinksPerLevel = 1024

	BlocksPerEpoch        = uint64(builtin2.ExpectedLeadersPerEpoch)
	BlockMessageLimit     = 512
	BlockGasLimit         = int64(100_000_000_000)
	BlockGasTarget        = int64(BlockGasLimit / 2)
	BaseFeeMaxChangeDenom = int64(8) // 12.5%
	InitialBaseFee        = int64(100e6)
	MinimumBaseFee        = int64(100)
	BlockDelaySecs        = uint64(builtin2.EpochDurationSeconds)
	PropagationDelaySecs  = uint64(6)
	EquivocationDelaySecs = uint64(2)
	SupportedProofTypes   = []abi.RegisteredSealProof{
		abi.RegisteredSealProof_StackedDrg32GiBV1,
		abi.RegisteredSealProof_StackedDrg64GiBV1,
	}
	ConsensusMinerMinPower  = abi.NewStoragePower(10 << 40)
	PreCommitChallengeDelay = abi.ChainEpoch(150)

	AllowableClockDriftSecs = uint64(1)

	SlashablePowerDelay        = 20
	InteractivePoRepConfidence = 6

	MessageConfidence uint64 = 5

	WRatioNum = int64(1)
	WRatioDen = uint64(2)

	BadBlockCacheSize     = 1 << 15
	BlsSignatureCacheSize = 40000
	VerifSigCacheSize     = 32000

	SealRandomnessLookback = policy.SealRandomnessLookback

	TicketRandomnessLookback = abi.ChainEpoch(1)

	FilBase               uint64 = 2_000_000_000
	FilAllocStorageMining uint64 = 1_400_000_000
	FilReserved           uint64 = 300_000_000

	FilecoinPrecision uint64 = 1_000_000_000_000_000_000

	InitialRewardBalance = func() *big.Int {
		v := big.NewInt(int64(FilAllocStorageMining))
		v = v.Mul(v, big.NewInt(int64(FilecoinPrecision)))
		return v
	}()

	InitialFilReserved = func() *big.Int {
		v := big.NewInt(int64(FilReserved))
		v = v.Mul(v, big.NewInt(int64(FilecoinPrecision)))
		return v
	}()

	// Actor consts
	// TODO: pieceSize unused from actors
	MinDealDuration, MaxDealDuration = policy.DealDurationBounds(0)

	PackingEfficiencyNum   int64 = 4
	PackingEfficiencyDenom int64 = 5

	UpgradeBreezeHeight      abi.ChainEpoch = -1
	BreezeGasTampingDuration abi.ChainEpoch = 0

	UpgradeSmokeHeight      abi.ChainEpoch = -1
	UpgradeIgnitionHeight   abi.ChainEpoch = -2
	UpgradeRefuelHeight     abi.ChainEpoch = -3
	UpgradeTapeHeight       abi.ChainEpoch = -4
	UpgradeAssemblyHeight   abi.ChainEpoch = 10
	UpgradeLiftoffHeight    abi.ChainEpoch = -5
	UpgradeKumquatHeight    abi.ChainEpoch = -6
	UpgradeCalicoHeight     abi.ChainEpoch = -8
	UpgradePersianHeight    abi.ChainEpoch = -9
	UpgradeOrangeHeight     abi.ChainEpoch = -10
	UpgradeClausHeight      abi.ChainEpoch = -11
	UpgradeTrustHeight      abi.ChainEpoch = -12
	UpgradeNorwegianHeight  abi.ChainEpoch = -13
	UpgradeTurboHeight      abi.ChainEpoch = -14
	UpgradeHyperdriveHeight abi.ChainEpoch = -15
	UpgradeChocolateHeight  abi.ChainEpoch = -16
	UpgradeOhSnapHeight     abi.ChainEpoch = -17
	UpgradeSkyrHeight       abi.ChainEpoch = -18
	UpgradeSharkHeight      abi.ChainEpoch = -19
	UpgradeHyggeHeight      abi.ChainEpoch = -20
	UpgradeLightningHeight  abi.ChainEpoch = -21
	UpgradeThunderHeight    abi.ChainEpoch = -22
	UpgradeWatermelonHeight abi.ChainEpoch = -23

	DrandSchedule = map[abi.ChainEpoch]DrandEnum{
		0: DrandMainnet,
	}

	GenesisNetworkVersion = network.Version0
	NetworkBundle         = "devnet"
	BundleOverrides       map[actorstypes.Version]string
	ActorDebugging        = true

	NewestNetworkVersion       = network.Version16
	ActorUpgradeNetworkVersion = network.Version16

	Devnet      = true
	ZeroAddress = MustParseAddress("f3yaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaby2smx7a")

	WhitelistedBlock  = cid.Undef
	BootstrappersFile = ""
	GenesisFile       = ""
)

const Finality = policy.ChainFinality
const ForkLengthThreshold = Finality

const BootstrapPeerThreshold = 1

// ChainId defines the chain ID used in the Ethereum JSON-RPC endpoint.
// As per https://github.com/ethereum-lists/chains
const Eip155ChainId = 31415926
