package build

import "math/big"

// Core network constants

// /////
// Storage

const UnixfsChunkSize uint64 = 1 << 20
const UnixfsLinksPerLevel = 1024

const SectorSize = 16 << 20

// /////
// Payments

// Blocks
const PaymentChannelClosingDelay = 6 * 60 * 2 // six hours

// Blocks
const DealVoucherSkewLimit = 10

// Blocks
const MinDealVoucherIncrement = ProvingPeriodDuration

const MaxVouchersPerDeal = 768 // roughly one voucher per 10h over a year

// /////
// Consensus / Network

// Seconds
const BlockDelay = 6

// Seconds
const AllowableClockDrift = BlockDelay * 2

// Blocks
const ForkLengthThreshold = 100

// /////
// Proofs / Mining

// Blocks
const RandomnessLookback = 20

// Blocks
const ProvingPeriodDuration = 40

// Blocks
const PoSTChallangeTime = 20

const PowerCollateralProportion = 20
const PerCapitaCollateralProportion = 5
const CollateralPrecision = 100

// /////
// Devnet settings

const TotalFilecoin = 2000000000
const MiningRewardTotal = 1400000000

const InitialRewardStr = "153856861913558700202"

var InitialReward *big.Int

const FilecoinPrecision = 1000000000000000000

// six years
// Blocks
const HalvingPeriodBlocks = 6 * 365 * 24 * 60 * 2

// Blocks
const AdjustmentPeriod = 7 * 24 * 60 * 2

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
const BadBlockCacheSize = 8192
