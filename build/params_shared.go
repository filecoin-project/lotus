package build

import (
	"math/big"
	"sort"

	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func DefaultSectorSize() abi.SectorSize {
	szs := make([]abi.SectorSize, 0, len(miner.SupportedProofTypes))
	for spt := range miner.SupportedProofTypes {
		ss, err := spt.SectorSize()
		if err != nil {
			panic(err)
		}

		szs = append(szs, ss)
	}

	sort.Slice(szs, func(i, j int) bool {
		return szs[i] < szs[j]
	})

	return szs[0]
}

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

// /////
// Consensus / Network

// Seconds
const AllowableClockDrift = 1

// Epochs
const ForkLengthThreshold = Finality

// Blocks (e)
var BlocksPerEpoch = uint64(builtin.ExpectedLeadersPerEpoch)

// Epochs
const Finality = miner.ChainFinalityish

// constants for Weight calculation
// The ratio of weight contributed by short-term vs long-term factors in a given round
const WRatioNum = int64(1)
const WRatioDen = 2

// /////
// Proofs

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

const WinningPoStSectorSetLookback = 10

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

// TODO: If this is gonna stay, it should move to specs-actors
const BlockMessageLimit = 512
const BlockGasLimit = 100_000_000

var DrandCoeffs = []string{
	"82c279cce744450e68de98ee08f9698a01dd38f8e3be3c53f2b840fb9d09ad62a0b6b87981e179e1b14bc9a2d284c985",
	"82d51308ad346c686f81b8094551597d7b963295cbf313401a93df9baf52d5ae98a87745bee70839a4d6e65c342bd15b",
	"94eebfd53f4ba6a3b8304236400a12e73885e5a781509a5c8d41d2e8b476923d8ea6052649b3c17282f596217f96c5de",
	"8dc4231e42b4edf39e86ef1579401692480647918275da767d3e558c520d6375ad953530610fd27daf110187877a65d0",
}
