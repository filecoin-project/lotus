package build

import (
	"math/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
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

var FilBase = buildconstants.FilBase                             // Deprecated: Use buildconstants.FilBase instead
var FilAllocStorageMining = buildconstants.FilAllocStorageMining // Deprecated: Use buildconstants.FilAllocStorageMining instead

var FilecoinPrecision = buildconstants.FilecoinPrecision // Deprecated: Use buildconstants.FilecoinPrecision instead
var FilReserved = buildconstants.FilReserved             // Deprecated: Use buildconstants.FilReserved instead

var InitialRewardBalance *big.Int
var InitialFilReserved *big.Int

func init() {
	InitialRewardBalance = big.NewInt(int64(FilAllocStorageMining))
	InitialRewardBalance = InitialRewardBalance.Mul(InitialRewardBalance, big.NewInt(int64(FilecoinPrecision)))

	InitialFilReserved = big.NewInt(int64(FilReserved))
	InitialFilReserved = InitialFilReserved.Mul(InitialFilReserved, big.NewInt(int64(FilecoinPrecision)))
}

// Sync
var BadBlockCacheSize = buildconstants.BadBlockCacheSize // Deprecated: Use buildconstants.BadBlockCacheSize instead

var BlsSignatureCacheSize = buildconstants.BlsSignatureCacheSize // Deprecated: Use buildconstants.BlsSignatureCacheSize instead

var VerifSigCacheSize = buildconstants.VerifSigCacheSize // Deprecated: Use buildconstants.VerifSigCacheSize instead

// ///////
// Limits

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

func init() {
	policy.SetSupportedProofTypes(buildconstants.SupportedProofTypes...)
	policy.SetConsensusMinerMinPower(buildconstants.ConsensusMinerMinPower)
	policy.SetPreCommitChallengeDelay(buildconstants.PreCommitChallengeDelay)
}
