// +build !testground

package build

import (
	"math/big"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// /////
// Storage

const UnixfsChunkSize uint64 = 1 << 20
const UnixfsLinksPerLevel = 1024

// /////
// Consensus / Network

const AllowableClockDriftSecs = uint64(1)

// Epochs
const ForkLengthThreshold = Finality

// Blocks (e)
var BlocksPerEpoch = uint64(builtin.ExpectedLeadersPerEpoch)

// Epochs
const Finality = miner.ChainFinalityish
const MessageConfidence = uint64(5)

// constants for Weight calculation
// The ratio of weight contributed by short-term vs long-term factors in a given round
const WRatioNum = int64(1)
const WRatioDen = uint64(2)

// /////
// Proofs

// Epochs
const SealRandomnessLookback = Finality

// Epochs
const SealRandomnessLookbackLimit = SealRandomnessLookback + 2000 // TODO: Get from spec specs-actors

// Maximum lookback that randomness can be sourced from for a seal proof submission
const MaxSealLookback = SealRandomnessLookbackLimit + 2000 // TODO: Get from specs-actors

// /////
// Mining

// Epochs
const TicketRandomnessLookback = abi.ChainEpoch(1)

const WinningPoStSectorSetLookback = abi.ChainEpoch(10)

// /////
// Devnet settings

const TotalFilecoin = uint64(2_000_000_000)
const MiningRewardTotal = uint64(1_400_000_000)

const FilecoinPrecision = uint64(1_000_000_000_000_000_000)

var InitialRewardBalance *big.Int

// TODO: Move other important consts here

func init() {
	InitialRewardBalance = big.NewInt(int64(MiningRewardTotal))
	InitialRewardBalance = InitialRewardBalance.Mul(InitialRewardBalance, big.NewInt(int64(FilecoinPrecision)))
}

// Sync
const BadBlockCacheSize = 1 << 15

// assuming 4000 messages per round, this lets us not lose any messages across a
// 10 block reorg.
const BlsSignatureCacheSize = 40000

// Size of signature verification cache
// 32k keeps the cache around 10MB in size, max
const VerifSigCacheSize = 32000

// ///////
// Limits

// TODO: If this is gonna stay, it should move to specs-actors
const BlockMessageLimit = 512
const BlockGasLimit = 100_000_000_000

var DrandConfig = dtypes.DrandConfig{
	Servers: []string{
		"https://pl-eu.testnet.drand.sh",
		"https://pl-us.testnet.drand.sh",
		"https://pl-sin.testnet.drand.sh",
	},
	ChainInfoJSON: `{"public_key":"922a2e93828ff83345bae533f5172669a26c02dc76d6bf59c80892e12ab1455c229211886f35bb56af6d5bea981024df","period":25,"genesis_time":1590445175,"hash":"138a324aa6540f93d0dad002aa89454b1bec2b6e948682cde6bd4db40f4b7c9b"}`,
}
