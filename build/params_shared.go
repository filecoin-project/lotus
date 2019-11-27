package build

import (
	"math/big"
)

// Core network constants

// /////
// Storage

const UnixfsChunkSize uint64 = 1 << 20
const UnixfsLinksPerLevel = 1024

var SectorSizes = []uint64{
	1 << 10,
	16 << 20,
	256 << 20,
	1 << 30,
}

const nodeSize = 32

// sector size -> winSzNodes
//
//  https://github.com/filecoin-project/rust-fil-proofs/blob/master/filecoin-proofs/src/constants.rs#L16-L20
var WindowSizeNodes = map[uint64]uint32{
	1 << 10: 512 / nodeSize,

	16 << 20:  (4 << 20) / nodeSize,
	256 << 20: (64 << 20) / nodeSize,

	1 << 30: (128 << 20) / nodeSize,
}

func SupportedSectorSize(ssize uint64) bool {
	for _, ss := range SectorSizes {
		if ssize == ss {
			return true
		}
	}
	return false
}

// /////
// Payments

// Blocks
const PaymentChannelClosingDelay = 6 * 60 * 2 // six hours

// /////
// Consensus / Network

// Seconds
const AllowableClockDrift = BlockDelay * 2

// Blocks
const ForkLengthThreshold = Finality

// Blocks (e)
const BlocksPerEpoch = 5

// Blocks
const Finality = 500

// constants for Weight calculation
// The ratio of weight contributed by short-term vs long-term factors in a given round
const WRatioNum = int64(1)
const WRatioDen = 2

// /////
// Proofs

// PoStChallangeTime sets the window in which post computation should happen
// Blocks
const PoStChallangeTime = ProvingPeriodDuration - 6

// PoStRandomnessLookback is additional randomness lookback for PoSt computation
// To compute randomness epoch in a given proving period:
// RandH = PPE - PoStChallangeTime - PoStRandomnessLookback
//
// Blocks
const PoStRandomnessLookback = 1

// Blocks
const SealRandomnessLookback = Finality

// Blocks
const SealRandomnessLookbackLimit = SealRandomnessLookback + 2000

// 1 / n
const SectorChallengeRatioDiv = 25

// /////
// Mining

// Blocks
const EcRandomnessLookback = 300

const FallbackPoStBegin = 1000
const SlashablePowerDelay = 2000

const PowerCollateralProportion = 5
const PerCapitaCollateralProportion = 1
const CollateralPrecision = 1000

// Blocks
const InteractivePoRepDelay = 10

// /////
// Devnet settings

const TotalFilecoin = 2_000_000_000
const MiningRewardTotal = 1_400_000_000

const InitialRewardStr = "153856861913558700202"

var InitialReward *big.Int

const FilecoinPrecision = 1_000_000_000_000_000_000

// six years
// Blocks
const HalvingPeriodBlocks = 6 * 365 * 24 * 60 * 2

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

// assuming 4000 blocks per round, this lets us not lose any messages across a
// 10 block reorg.
const BlsSignatureCacheSize = 40000
