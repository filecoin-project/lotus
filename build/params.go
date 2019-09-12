package build

// Core network constants

const UnixfsChunkSize uint64 = 1 << 20
const UnixfsLinksPerLevel = 1024

const SectorSize = 1024

const PaymentChannelClosingDelay = 6 * 60 * 2 // six hours

const DealVoucherSkewLimit = 10

const ForkLengthThreshold = 20
const RandomnessLookback = 20

const PowerCollateralProportion = 0.2
const PerCapitaCollateralProportion = 0.05

// TODO: Move other important consts here
