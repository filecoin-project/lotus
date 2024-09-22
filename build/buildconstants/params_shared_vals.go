//go:build !testground
// +build !testground

package buildconstants

import (
	"math/big"
	"os"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

// /////
// Consensus / Network

func init() {
	policy.SetSupportedProofTypes(SupportedProofTypes...)
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
const FilAllocStorageMining = uint64(1_100_000_000)

const FilecoinPrecision = uint64(1_000_000_000_000_000_000)
const FilReserved = uint64(300_000_000)

var InitialRewardBalance *big.Int
var InitialFilReserved *big.Int

func init() {
	InitialRewardBalance = big.NewInt(int64(FilAllocStorageMining))
	InitialRewardBalance = InitialRewardBalance.Mul(InitialRewardBalance, big.NewInt(int64(FilecoinPrecision)))

	InitialFilReserved = big.NewInt(int64(FilReserved))
	InitialFilReserved = InitialFilReserved.Mul(InitialFilReserved, big.NewInt(int64(FilecoinPrecision)))

	if os.Getenv("LOTUS_ADDRESS_TYPE") == AddressMainnetEnvVar {
		SetAddressNetwork(address.Mainnet)
	}

	if ptCid := os.Getenv("F3_INITIAL_POWERTABLE_CID"); ptCid != "" {
		if k, err := cid.Parse(ptCid); err != nil {
			log.Errorf("failed to parse F3_INITIAL_POWERTABLE_CID %q: %s", ptCid, err)
		} else if F3InitialPowerTableCID.Defined() && k != F3InitialPowerTableCID {
			log.Errorf("ignoring F3_INITIAL_POWERTABLE_CID as lotus has a hard-coded initial F3 power table")
		} else {
			F3InitialPowerTableCID = k
		}
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
