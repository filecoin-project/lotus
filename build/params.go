package build

// Core network constants

const UnixfsChunkSize uint64 = 1 << 20
const UnixfsLinksPerLevel = 1024

const SectorSize = 1024

const PaymentChannelClosingDelay = 6 * 60 * 2 // six hours

const DealVoucherSkewLimit = 10

const ForkLengthThreshold = 20
const RandomnessLookback = 20

const ProvingPeriodDuration = 2 * 60 // an hour, for now
const PoSTChallangeTime = 1 * 60

// TODO: Move other important consts here
