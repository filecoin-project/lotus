package build

import (
	"math/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

// /////
// Storage

var UnixfsChunkSize uint64 = buildconstants.UnixfsChunkSize
var UnixfsLinksPerLevel = buildconstants.UnixfsLinksPerLevel

// /////
// Consensus / Network

var AllowableClockDriftSecs = buildconstants.AllowableClockDriftSecs

// Epochs
var ForkLengthThreshold = Finality

// Blocks (e)
var BlocksPerEpoch = buildconstants.BlocksPerEpoch

// Epochs
var MessageConfidence = buildconstants.MessageConfidence

// constants for Weight calculation
// The ratio of weight contributed by short-term vs long-term factors in a given round
var WRatioNum = buildconstants.WRatioNum
var WRatioDen = buildconstants.WRatioDen

// /////
// Mining

// Epochs
var TicketRandomnessLookback = buildconstants.TicketRandomnessLookback

// the 'f' prefix doesn't matter
var ZeroAddress = buildconstants.ZeroAddress

// /////
// Devnet settings

var Devnet = buildconstants.Devnet

var FilBase = buildconstants.FilBase
var FilAllocStorageMining = buildconstants.FilAllocStorageMining

var FilecoinPrecision = buildconstants.FilecoinPrecision
var FilReserved = buildconstants.FilReserved

var InitialRewardBalance *big.Int
var InitialFilReserved *big.Int

// TODO: Move other important consts here

func init() {
	InitialRewardBalance = big.NewInt(int64(FilAllocStorageMining))
	InitialRewardBalance = InitialRewardBalance.Mul(InitialRewardBalance, big.NewInt(int64(FilecoinPrecision)))

	InitialFilReserved = big.NewInt(int64(FilReserved))
	InitialFilReserved = InitialFilReserved.Mul(InitialFilReserved, big.NewInt(int64(FilecoinPrecision)))
}

// Sync
var BadBlockCacheSize = buildconstants.BadBlockCacheSize

// assuming 4000 messages per round, this lets us not lose any messages across a
// 10 block reorg.
var BlsSignatureCacheSize = buildconstants.BlsSignatureCacheSize

// Size of signature verification cache
// 32k keeps the cache around 10MB in size, max
var VerifSigCacheSize = buildconstants.VerifSigCacheSize

// ///////
// Limits

// TODO: If this is gonna stay, it should move to specs-actors
var BlockMessageLimit = buildconstants.BlockMessageLimit

var BlockGasLimit = buildconstants.BlockGasLimit
var BlockGasTarget = buildconstants.BlockGasTarget

var BaseFeeMaxChangeDenom int64 = buildconstants.BaseFeeMaxChangeDenom
var InitialBaseFee int64 = buildconstants.InitialBaseFee
var MinimumBaseFee int64 = buildconstants.MinimumBaseFee
var PackingEfficiencyNum int64 = buildconstants.PackingEfficiencyNum
var PackingEfficiencyDenom int64 = buildconstants.PackingEfficiencyDenom

var MinDealDuration = buildconstants.MinDealDuration
var MaxDealDuration = buildconstants.MaxDealDuration

const TestNetworkVersion = buildconstants.TestNetworkVersion
