//go:build !testground

package buildconstants

import (
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

// /////
// Consensus / Network

func init() {
	policy.SetConsensusMinerMinPower(ConsensusMinerMinPower)
	policy.SetPreCommitChallengeDelay(PreCommitChallengeDelay)
}

const AllowableClockDriftSecs = uint64(1)

// Blocks (e)
var BlocksPerEpoch = uint64(builtin2.ExpectedLeadersPerEpoch)

// Epochs
const MessageConfidence = uint64(5)

// constants for Weight calculation
// The ratio of weight contributed by short-term vs long-term factors in a given round
const WRatioNum = int64(1)
const WRatioDen = uint64(2)

// /////
// Proofs

// Epochs
// TODO: unused
const SealRandomnessLookback = policy.SealRandomnessLookback

// /////
// Mining

// Epochs
const TicketRandomnessLookback = abi.ChainEpoch(1)

// /////
// Address

const AddressMainnetEnvVar = "_mainnet_"

// the 'f' prefix doesn't matter
var ZeroAddress = MustParseAddress("f3yaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaby2smx7a")

const FilBase = uint64(2_000_000_000)
const FilecoinPrecision = uint64(1_000_000_000_000_000_000)

var InitialRewardBalance = wholeFIL(1_100_000_000)
var InitialFilReserved = wholeFIL(300_000_000)

func init() {
	if os.Getenv("LOTUS_ADDRESS_TYPE") == AddressMainnetEnvVar {
		SetAddressNetwork(address.Mainnet)
	}
}

// Sync
const BadBlockCacheSize = 1 << 15

// assuming 4000 messages per round, this lets us not lose any messages across a
// 10 block reorg.
const BlsSignatureCacheSize = 40000

// Size of signature verification cache
// 32k keeps the cache around 10MB in size, max
const VerifSigCacheSize = 32000

// ///////
// Limits

const BlockMessageLimit = 10000

var BlockGasLimit = int64(10_000_000_000)
var BlockGasTarget = BlockGasLimit / 2

const BaseFeeMaxChangeDenom int64 = 8 // 12.5%
const InitialBaseFee int64 = 100e6
const MinimumBaseFee int64 = 100
const PackingEfficiencyNum int64 = 4
const PackingEfficiencyDenom int64 = 5

// SafeHeightDistance is the distance from the latest tipset, i.e. heaviest, that
// is considered to be safe from re-orgs at an increasingly diminishing
// probability.
//
// This is used to determine the safe tipset when using the "safe" tag in
// TipSetSelector or via Eth JSON-RPC APIs. Note that "safe" doesn't guarantee
// finality, but rather a high probability of not being reverted. For guaranteed
// finality, use the "finalized" tag.
//
// This constant is experimental and may change in the future.
// Discussion on this current value and a tracking item to document the
// probabilistic impact of various values is in
// https://github.com/filecoin-project/go-f3/issues/944
const SafeHeightDistance abi.ChainEpoch = 200
