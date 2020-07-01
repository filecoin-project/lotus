// +build testground

// This file makes hardcoded parameters (const) configurable as vars.
//
// Its purpose is to unlock various degrees of flexibility and parametrization
// when writing Testground plans for Lotus.
//
package build

import (
	"math/big"

	"github.com/filecoin-project/lotus/node/modules/dtypes"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

var (
	UnixfsChunkSize     = uint64(1 << 20)
	UnixfsLinksPerLevel = 1024

	BlocksPerEpoch       = uint64(builtin.ExpectedLeadersPerEpoch)
	BlockMessageLimit    = 512
	BlockGasLimit        = int64(100_000_000_000)
	BlockDelaySecs       = uint64(builtin.EpochDurationSeconds)
	PropagationDelaySecs = uint64(6)

	AllowableClockDriftSecs = uint64(1)

	Finality            = miner.ChainFinalityish
	ForkLengthThreshold = Finality

	SlashablePowerDelay        = 20
	InteractivePoRepConfidence = 6

	MessageConfidence uint64 = 5

	WRatioNum = int64(1)
	WRatioDen = uint64(2)

	BadBlockCacheSize     = 1 << 15
	BlsSignatureCacheSize = 40000
	VerifSigCacheSize     = 32000

	SealRandomnessLookback      = Finality
	SealRandomnessLookbackLimit = SealRandomnessLookback + 2000
	MaxSealLookback             = SealRandomnessLookbackLimit + 2000

	TicketRandomnessLookback     = abi.ChainEpoch(1)
	WinningPoStSectorSetLookback = abi.ChainEpoch(10)

	TotalFilecoin     uint64 = 2_000_000_000
	MiningRewardTotal uint64 = 1_400_000_000

	FilecoinPrecision uint64 = 1_000_000_000_000_000_000

	InitialRewardBalance = func() *big.Int {
		v := big.NewInt(int64(MiningRewardTotal))
		v = v.Mul(v, big.NewInt(int64(FilecoinPrecision)))
		return v
	}()

	DrandConfig = dtypes.DrandConfig{
		Servers: []string{
			"https://pl-eu.testnet.drand.sh",
			"https://pl-us.testnet.drand.sh",
			"https://pl-sin.testnet.drand.sh",
		},
		ChainInfoJSON: `{"public_key":"922a2e93828ff83345bae533f5172669a26c02dc76d6bf59c80892e12ab1455c229211886f35bb56af6d5bea981024df","period":25,"genesis_time":1590445175,"hash":"138a324aa6540f93d0dad002aa89454b1bec2b6e948682cde6bd4db40f4b7c9b"}`,
	}
)
