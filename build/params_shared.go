package build

import (
	"math/big"
)

// Core network constants

// /////
// Storage

const UnixfsChunkSize uint64 = 1 << 20
const UnixfsLinksPerLevel = 1024

func SupportedSectorSize(ssize uint64) bool {
	for _, ss := range SectorSizes {
		if ssize == ss {
			return true
		}
	}
	return false
}

const SectorChallengeRatioDiv = 25

// /////
// Payments

// Epochs
const PaymentChannelClosingDelay = 6 * 60 * 60 / BlockDelay // six hours

// /////
// Consensus / Network

// Seconds
const AllowableClockDrift = 1

// Epochs
const ForkLengthThreshold = Finality

// Blocks (e)
const BlocksPerEpoch = 5

// Epochs
const Finality = 500

// constants for Weight calculation
// The ratio of weight contributed by short-term vs long-term factors in a given round
const WRatioNum = int64(1)
const WRatioDen = 2

// /////
// Proofs

// Epochs
const SealRandomnessLookback = Finality

// Epochs
const SealRandomnessLookbackLimit = SealRandomnessLookback + 2000

// Maximum lookback that randomness can be sourced from for a seal proof submission
const MaxSealLookback = SealRandomnessLookbackLimit + 2000

// /////
// Mining

// Epochs
const EcRandomnessLookback = 300

const PowerCollateralProportion = 5
const PerCapitaCollateralProportion = 1
const CollateralPrecision = 1000

// /////
// Devnet settings

const TotalFilecoin = 2_000_000_000
const MiningRewardTotal = 1_400_000_000

const InitialRewardStr = "153856861913558700202"

var InitialReward *big.Int

const FilecoinPrecision = 1_000_000_000_000_000_000

// TODO: Move other important consts here

func init() {
	InitialReward = new(big.Int)

	var ok bool
	InitialReward, ok = InitialReward.
		SetString(InitialRewardStr, 10)
	if !ok {
		panic("could not parse InitialRewardStr")
	}
}

// Sync
const BadBlockCacheSize = 1 << 15

// assuming 4000 messages per round, this lets us not lose any messages across a
// 10 block reorg.
const BlsSignatureCacheSize = 40000

// ///////
// Limits

const BlockMessageLimit = 512
const MinerMaxSectors = 1 << 48
