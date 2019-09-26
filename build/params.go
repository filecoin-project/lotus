package build

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
const BlockDelay = 4

// Seconds
const AllowableClockDrift = BlockDelay * 2

// Blocks
const ForkLengthThreshold = 20

// /////
// Proofs / Mining

// Blocks
const RandomnessLookback = 20

// Blocks
const ProvingPeriodDuration = 10

// Blocks
const PoSTChallangeTime = 5

const PowerCollateralProportion = 20
const PerCapitaCollateralProportion = 5
const CollateralPrecision = 100

// /////
// Devnet settings

const TotalFilecoin = 2000000000
const FilecoinPrecision = 1000000000000000000

// TODO: Move other important consts here
