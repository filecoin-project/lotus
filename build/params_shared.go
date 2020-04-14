package build

import (
	"math/big"

	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
	sabig "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
)

// Core network constants

func BlocksTopic(netName dtypes.NetworkName) string   { return "/fil/blocks/" + string(netName) }
func MessagesTopic(netName dtypes.NetworkName) string { return "/fil/msgs/" + string(netName) }
func DhtProtocolName(netName dtypes.NetworkName) protocol.ID {
	return protocol.ID("/fil/kad/" + string(netName))
}

// /////
// Storage

const UnixfsChunkSize uint64 = 1 << 20
const UnixfsLinksPerLevel = 1024

const SectorChallengeRatioDiv = 25

// /////
// Payments

// Epochs
const PaymentChannelClosingDelay = 6 * 60 * 60 / BlockDelay // six hours

// /////
// Consensus / Network

// Seconds
const AllowableClockDrift = 1

// Epochs
const ForkLengthThreshold = Finality

// Blocks (e)
const BlocksPerEpoch = 5

// Epochs
const Finality = 500

// constants for Weight calculation
// The ratio of weight contributed by short-term vs long-term factors in a given round
const WRatioNum = int64(1)
const WRatioDen = 2

// /////
// Proofs

// Epochs
const FallbackPoStConfidence = 6

// Epochs
const SealRandomnessLookback = Finality

// Epochs
const SealRandomnessLookbackLimit = SealRandomnessLookback + 2000 // TODO: Get from spec specs-actors

// Maximum lookback that randomness can be sourced from for a seal proof submission
const MaxSealLookback = SealRandomnessLookbackLimit + 2000 // TODO: Get from specs-actors

// /////
// Mining

// Epochs
const TicketRandomnessLookback = 1

// /////
// Devnet settings

const TotalFilecoin = 2_000_000_000
const MiningRewardTotal = 1_400_000_000

const FilecoinPrecision = 1_000_000_000_000_000_000

var InitialRewardBalance *big.Int

// TODO: Move other important consts here

func init() {
	InitialRewardBalance = big.NewInt(MiningRewardTotal)
	InitialRewardBalance = InitialRewardBalance.Mul(InitialRewardBalance, big.NewInt(FilecoinPrecision))

	power.ConsensusMinerMinPower = sabig.NewInt(2048)
}

// Sync
const BadBlockCacheSize = 1 << 15

// assuming 4000 messages per round, this lets us not lose any messages across a
// 10 block reorg.
const BlsSignatureCacheSize = 40000

// ///////
// Limits

const BlockMessageLimit = 512

var DrandCoeffs = []string{
	"a2a34cf9a6be2f66b5385caa520364f994ae7dbac08371ffaca575dfb3e04c8e149b32dc78f077322c613a151dc07440",
	"b0c5baca062191f13099229c9a229a9946204f74fc28baa212745243553ab1f50b581b2086e24374ceb40fe34bd23ca2",
	"a9c6449cf647e0a0ffaf1e01277e2821213c80310165990daf77610208abfa0ce56c7e40995e26aff3873c624362ca78",
}
