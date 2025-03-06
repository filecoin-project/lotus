package build

import (
	"github.com/filecoin-project/lotus/build/buildconstants"
)

// /////
// Consensus / Network

var AllowableClockDriftSecs = buildconstants.AllowableClockDriftSecs // Deprecated: Use buildconstants.AllowableClockDriftSecs instead

// Epochs
const ForkLengthThreshold = Finality // Deprecated: Use Finality instead

// Blocks (e)
var BlocksPerEpoch = buildconstants.BlocksPerEpoch // Deprecated: Use buildconstants.BlocksPerEpoch instead

// Epochs
var MessageConfidence = buildconstants.MessageConfidence // Deprecated: Use buildconstants.MessageConfidence instead

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

var FilBase = buildconstants.FilBase // Deprecated: Use buildconstants.FilBase instead

var FilecoinPrecision = buildconstants.FilecoinPrecision // Deprecated: Use buildconstants.FilecoinPrecision instead

var InitialRewardBalance = buildconstants.InitialRewardBalance // Deprecated: Use buildconstants.InitialRewardBalance instead
var InitialFilReserved = buildconstants.InitialFilReserved     // Deprecated: Use buildconstants.InitialFilReserved instead

// Sync
var BadBlockCacheSize = buildconstants.BadBlockCacheSize // Deprecated: Use buildconstants.BadBlockCacheSize instead

var BlsSignatureCacheSize = buildconstants.BlsSignatureCacheSize // Deprecated: Use buildconstants.BlsSignatureCacheSize instead

var VerifSigCacheSize = buildconstants.VerifSigCacheSize // Deprecated: Use buildconstants.VerifSigCacheSize instead

// ///////
// Limits

var BlockMessageLimit = buildconstants.BlockMessageLimit // Deprecated: Use buildconstants.BlockMessageLimit instead

var BlockGasLimit = buildconstants.BlockGasLimit   // Deprecated: Use buildconstants.BlockGasLimit instead
var BlockGasTarget = buildconstants.BlockGasTarget // Deprecated: Use buildconstants.BlockGasTarget instead

var BaseFeeMaxChangeDenom = buildconstants.BaseFeeMaxChangeDenom   // Deprecated: Use buildconstants.BaseFeeMaxChangeDenom instead
var InitialBaseFee = buildconstants.InitialBaseFee                 // Deprecated: Use buildconstants.InitialBaseFee instead
var MinimumBaseFee = buildconstants.MinimumBaseFee                 // Deprecated: Use buildconstants.MinimumBaseFee instead
var PackingEfficiencyNum = buildconstants.PackingEfficiencyNum     // Deprecated: Use buildconstants.PackingEfficiencyNum instead
var PackingEfficiencyDenom = buildconstants.PackingEfficiencyDenom // Deprecated: Use buildconstants.PackingEfficiencyDenom instead

const TestNetworkVersion = buildconstants.TestNetworkVersion // Deprecated: Use buildconstants.TestNetworkVersion instead

var MinerFDLimit = buildconstants.MinerFDLimit // Deprecated: Use buildconstants.MinerFDLimit instead
