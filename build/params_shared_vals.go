package build

import (
	"math/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

// /////
// Storage

var UnixfsChunkSize uint64 = buildconstants.UnixfsChunkSize  // Deprecated: Use buildconstants.UnixfsChunkSize instead
var UnixfsLinksPerLevel = buildconstants.UnixfsLinksPerLevel // Deprecated: Use buildconstants.UnixfsLinksPerLevel instead

// /////
// Consensus / Network

var AllowableClockDriftSecs = buildconstants.AllowableClockDriftSecs // Deprecated: Use buildconstants.AllowableClockDriftSecs instead

// Epochs
var ForkLengthThreshold = Finality // Deprecated: Use Finality instead

// Blocks (e)
var BlocksPerEpoch = buildconstants.BlocksPerEpoch // Deprecated: Use buildconstants.BlocksPerEpoch instead

// Epochs
var MessageConfidence = buildconstants.MessageConfidence // Deprecated: Use buildconstants.MessageConfidence instead

// constants for Weight calculation
// The ratio of weight contributed by short-term vs long-term factors in a given round
var WRatioNum = buildconstants.WRatioNum // Deprecated: Use buildconstants.WRatioNum instead
var WRatioDen = buildconstants.WRatioDen // Deprecated: Use buildconstants.WRatioDen instead

// /////
// Mining

// Epochs
var TicketRandomnessLookback = buildconstants.TicketRandomnessLookback // Deprecated: Use buildconstants.TicketRandomnessLookback instead

// the 'f' prefix doesn't matter
var ZeroAddress = buildconstants.ZeroAddress // Deprecated: Use buildconstants.ZeroAddress instead

// /////
// Devnet settings

var Devnet = buildconstants.Devnet // Deprecated: Use buildconstants.Devnet instead

var FilBase = buildconstants.FilBase                             // Deprecated: Use buildconstants.FilBase instead
var FilAllocStorageMining = buildconstants.FilAllocStorageMining // Deprecated: Use buildconstants.FilAllocStorageMining instead

var FilecoinPrecision = buildconstants.FilecoinPrecision // Deprecated: Use buildconstants.FilecoinPrecision instead
var FilReserved = buildconstants.FilReserved             // Deprecated: Use buildconstants.FilReserved instead

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
var BadBlockCacheSize = buildconstants.BadBlockCacheSize // Deprecated: Use buildconstants.BadBlockCacheSize instead

// assuming 4000 messages per round, this lets us not lose any messages across a
// 10 block reorg.
var BlsSignatureCacheSize = buildconstants.BlsSignatureCacheSize // Deprecated: Use buildconstants.BlsSignatureCacheSize instead

// Size of signature verification cache
// 32k keeps the cache around 10MB in size, max
var VerifSigCacheSize = buildconstants.VerifSigCacheSize // Deprecated: Use buildconstants.VerifSigCacheSize instead

// ///////
// Limits

// TODO: If this is gonna stay, it should move to specs-actors
var BlockMessageLimit = buildconstants.BlockMessageLimit // Deprecated: Use buildconstants.BlockMessageLimit instead

var BlockGasLimit = buildconstants.BlockGasLimit   // Deprecated: Use buildconstants.BlockGasLimit instead
var BlockGasTarget = buildconstants.BlockGasTarget // Deprecated: Use buildconstants.BlockGasTarget instead

var BaseFeeMaxChangeDenom int64 = buildconstants.BaseFeeMaxChangeDenom   // Deprecated: Use buildconstants.BaseFeeMaxChangeDenom instead
var InitialBaseFee int64 = buildconstants.InitialBaseFee                 // Deprecated: Use buildconstants.InitialBaseFee instead
var MinimumBaseFee int64 = buildconstants.MinimumBaseFee                 // Deprecated: Use buildconstants.MinimumBaseFee instead
var PackingEfficiencyNum int64 = buildconstants.PackingEfficiencyNum     // Deprecated: Use buildconstants.PackingEfficiencyNum instead
var PackingEfficiencyDenom int64 = buildconstants.PackingEfficiencyDenom // Deprecated: Use buildconstants.PackingEfficiencyDenom instead

var MinDealDuration = buildconstants.MinDealDuration // Deprecated: Use buildconstants.MinDealDuration instead
var MaxDealDuration = buildconstants.MaxDealDuration // Deprecated: Use buildconstants.MaxDealDuration instead

const TestNetworkVersion = buildconstants.TestNetworkVersion // Deprecated: Use buildconstants.TestNetworkVersion instead
