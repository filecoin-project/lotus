package v2api

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

// RewardDistribution describes the rewards allocated while executing one tipset.
// All token amounts are in attoFIL and serialize as decimal strings.
type RewardDistribution struct {
	TipSetKey types.TipSetKey
	Height    abi.ChainEpoch
	// Denom is the common denominator for stream weights and recipient shares.
	Denom uint64 `json:",string" jsonschema:"type=string"`
	// Totals sums the corresponding Amounts fields across Blocks.
	Totals RewardAmounts
	// Blocks follows the tipset's execution order.
	Blocks []BlockReward
}

// BlockReward describes one block's reward award and the distribution it used.
type BlockReward struct {
	Block    cid.Cid
	Miner    address.Address
	WinCount int64
	Amounts  RewardAmounts
	// BurnWeight is the fraction not assigned to any stream. BurnAllocation also
	// includes integer rounding and burn shares within explicit streams.
	BurnWeight uint64 `json:",string" jsonschema:"type=string"`
	Streams    []StreamReward
}

// RewardAmounts separates reward allocations from transfers. It describes one
// block when used in BlockReward.Amounts and their sum in RewardDistribution.Totals.
// MintedReward = MinerReward + ExplicitReward + BurnAllocation.
type RewardAmounts struct {
	// MintedReward is the block subsidy released from the reward actor's reserve.
	// A block with a positive WinCount and zero MintedReward received only its gas
	// reward: the reward actor's fallback when it cannot award normally, because
	// its reserve is exhausted or a stream accounting invariant failed. This should
	// not happen on a healthy network and warrants investigating the reward actor.
	MintedReward abi.TokenAmount
	// MinerReward is the minted reward allocated to the implicit miner stream.
	MinerReward abi.TokenAmount
	// MessageReward is the reward funded by message fees and allocated to the miner.
	MessageReward abi.TokenAmount
	// ExplicitReward is the minted reward retained for explicit stream recipients.
	ExplicitReward abi.TokenAmount
	// BurnAllocation is the minted reward allocated to burn by the distribution.
	BurnAllocation abi.TokenAmount
	// MinerPaid is the amount successfully transferred to miner actors, including
	// message rewards. It does not describe the miners' withdrawable balances.
	MinerPaid abi.TokenAmount
	// BurnPaid is the amount successfully transferred to the burnt funds actor by
	// reward awards, including prior-period dust settled during the award and any
	// miner payment redirected to burn on failure. It excludes burns from separate
	// messages and nested transfers from miner actors.
	BurnPaid abi.TokenAmount
}

// StreamReward is a stream's fraction and allocation for one block's award.
type StreamReward struct {
	ID     uint64 `json:",string" jsonschema:"type=string"`
	Weight uint64 `json:",string" jsonschema:"type=string"`
	// Amount is the gross minted reward allocated to this stream.
	Amount abi.TokenAmount
	// Distribution is null for the implicit stream, whose recipient is the block's
	// Miner. Explicit streams include their recipient shares and earnings.
	Distribution *ExplicitRewardDistribution
}

// ExplicitRewardDistribution describes the recipients of an explicit stream's
// award. Amount = sum(Recipients.EarnedAmount) + BurnAmount + RoundingAdjustment,
// where Amount is the containing StreamReward.Amount.
type ExplicitRewardDistribution struct {
	Writer     address.Address
	Recipients []RecipientReward
	// BurnShare is the fraction of the stream not assigned to recipients.
	BurnShare uint64 `json:",string" jsonschema:"type=string"`
	// BurnAmount is the stream allocation burned for this award.
	BurnAmount abi.TokenAmount
	// RoundingAdjustment reconciles the stream allocation with recipient earnings
	// and burn. It can be negative because recipient earnings are differences of
	// rounded cumulative entitlements, not independently rounded block amounts.
	// It is not an additional payment or burn.
	RoundingAdjustment abi.TokenAmount
}

// RecipientReward is one recipient's share and earnings from a stream award.
type RecipientReward struct {
	Recipient address.Address
	Share     uint64 `json:",string" jsonschema:"type=string"`
	// EarnedAmount is the increase in the recipient's entitlement from this award.
	// It does not include earnings from earlier awards or subtract later claims.
	EarnedAmount abi.TokenAmount
}
