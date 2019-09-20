package build

// Core network constants

const UnixfsChunkSize uint64 = 1 << 20
const UnixfsLinksPerLevel = 1024

const SectorSize = 1024

// Blocks
const PaymentChannelClosingDelay = 6 * 60 * 2 // six hours

// Blocks
const DealVoucherSkewLimit = 10

// Blocks
const ForkLengthThreshold = 20

// Blocks
const RandomnessLookback = 20

// Blocks
const ProvingPeriodDuration = 10
const PoSTChallangeTime = 5

// TODO: Move other important consts here
