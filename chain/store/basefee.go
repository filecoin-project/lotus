package store

import (
	"context"
	"math/rand"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

func ComputeNextBaseFee(baseFee types.BigInt, gasLimitUsed int64, noOfBlocks int, epoch abi.ChainEpoch) types.BigInt {
	// delta := gasLimitUsed/noOfBlocks - buildconstants.BlockGasTarget
	// change := baseFee * delta / BlockGasTarget
	// nextBaseFee = baseFee + change
	// nextBaseFee = max(nextBaseFee, buildconstants.MinimumBaseFee)

	var delta int64
	if epoch > buildconstants.UpgradeSmokeHeight {
		delta = gasLimitUsed / int64(noOfBlocks)
		delta -= buildconstants.BlockGasTarget
	} else {
		delta = buildconstants.PackingEfficiencyDenom * gasLimitUsed / (int64(noOfBlocks) * buildconstants.PackingEfficiencyNum)
		delta -= buildconstants.BlockGasTarget
	}

	// cap change at 12.5% (BaseFeeMaxChangeDenom) by capping delta
	if delta > buildconstants.BlockGasTarget {
		delta = buildconstants.BlockGasTarget
	}
	if delta < -buildconstants.BlockGasTarget {
		delta = -buildconstants.BlockGasTarget
	}

	change := big.Mul(baseFee, big.NewInt(delta))
	change = big.Div(change, big.NewInt(buildconstants.BlockGasTarget))
	change = big.Div(change, big.NewInt(buildconstants.BaseFeeMaxChangeDenom))

	nextBaseFee := big.Add(baseFee, change)
	if big.Cmp(nextBaseFee, big.NewInt(buildconstants.MinimumBaseFee)) < 0 {
		nextBaseFee = big.NewInt(buildconstants.MinimumBaseFee)
	}
	return nextBaseFee
}

func (cs *ChainStore) ComputeBaseFee(ctx context.Context, ts *types.TipSet) (abi.TokenAmount, error) {
	if buildconstants.UpgradeBreezeHeight >= 0 && ts.Height() > buildconstants.UpgradeBreezeHeight && ts.Height() < buildconstants.UpgradeBreezeHeight+buildconstants.BreezeGasTampingDuration {
		return abi.NewTokenAmount(100), nil
	}
	if ts.Height() < buildconstants.UpgradeXxHeight {
		return cs.ComputeNextBaseFeeFromUtilization(ctx, ts)
	}
	return cs.ComputeNextBaseFeeFromPremiums(ctx, ts)
}

func (cs *ChainStore) ComputeNextBaseFeeFromUtilization(ctx context.Context, ts *types.TipSet) (abi.TokenAmount, error) {
	zero := abi.NewTokenAmount(0)

	// totalLimit is sum of GasLimits of unique messages in a tipset
	totalLimit := int64(0)

	seen := make(map[cid.Cid]struct{})

	for _, b := range ts.Blocks() {
		blsMsgs, secpMsgs, err := cs.MessagesForBlock(ctx, b)
		if err != nil {
			return zero, xerrors.Errorf("error getting messages for: %s: %w", b.Cid(), err)
		}
		for _, m := range blsMsgs {
			c := m.Cid()
			if _, ok := seen[c]; !ok {
				totalLimit += m.GasLimit
				seen[c] = struct{}{}
			}
		}
		for _, m := range secpMsgs {
			c := m.Cid()
			if _, ok := seen[c]; !ok {
				totalLimit += m.Message.GasLimit
				seen[c] = struct{}{}
			}
		}
	}
	parentBaseFee := ts.Blocks()[0].ParentBaseFee

	return ComputeNextBaseFee(parentBaseFee, totalLimit, len(ts.Blocks()), ts.Height()), nil
}

type SenderNonce struct {
	Sender address.Address
	Nonce  uint64
}

func (cs *ChainStore) ComputeNextBaseFeeFromPremiums(ctx context.Context, ts *types.TipSet) (abi.TokenAmount, error) {
	zero := abi.NewTokenAmount(0)
	parentBaseFee := ts.Blocks()[0].ParentBaseFee
	var premiums []abi.TokenAmount
	var limits []int64
	seen := make(map[SenderNonce]struct{})

	for _, b := range ts.Blocks() {
		blsMsgs, secpMsgs, err := cs.MessagesForBlock(ctx, b)
		if err != nil {
			return zero, xerrors.Errorf("error getting messages for: %s: %w", b.Cid(), err)
		}
		for _, msg := range blsMsgs {
			senderNonce := SenderNonce{msg.From, msg.Nonce}
			if _, ok := seen[senderNonce]; !ok {
				limits = append(limits, msg.GasLimit)
				premiums = append(premiums, msg.EffectiveGasPremium(parentBaseFee))
				seen[senderNonce] = struct{}{}
			}
		}
		for _, signed := range secpMsgs {
			senderNonce := SenderNonce{signed.Message.From, signed.Message.Nonce}
			if _, ok := seen[senderNonce]; !ok {
				limits = append(limits, signed.Message.GasLimit)
				premiums = append(premiums, signed.Message.EffectiveGasPremium(parentBaseFee))
				seen[senderNonce] = struct{}{}
			}
		}
	}

	percentilePremium := WeightedQuickSelect(premiums, limits, buildconstants.BlockGasTargetIndex)

	return nextBaseFeeFromPremium(parentBaseFee, percentilePremium), nil
}

func nextBaseFeeFromPremium(baseFee, premiumP abi.TokenAmount) abi.TokenAmount {
	denom := big.NewInt(buildconstants.BaseFeeMaxChangeDenom)
	maxAdj := big.Div(big.Add(baseFee, big.Sub(denom, big.NewInt(1))), denom)
	return big.Max(
		big.NewInt(buildconstants.MinimumBaseFee),
		big.Add(
			baseFee,
			big.Min(maxAdj, big.Sub(premiumP, maxAdj)),
		),
	)
}

func WeightedQuickSelect(premiums []abi.TokenAmount, limits []int64, index int64) abi.TokenAmount {
	if len(premiums) == 1 {
		if limits[0] <= index {
			return big.Zero()
		}
		return premiums[0]
	}

	pivot := premiums[rand.Intn(len(premiums))]

	var less []abi.TokenAmount
	var lessWeights []int64
	var lessW int64
	var eqW int64
	var more []abi.TokenAmount
	var moreWeights []int64
	var moreW int64

	for i, premium := range premiums {
		cmp := big.Cmp(premium, pivot)
		if cmp < 0 {
			less = append(less, premium)
			lessWeights = append(lessWeights, limits[i])
			lessW += limits[i]
		} else if cmp == 0 {
			eqW += limits[i]
		} else {
			more = append(more, premium)
			moreWeights = append(moreWeights, limits[i])
			moreW += limits[i]
		}
	}

	if index < moreW {
		return WeightedQuickSelect(more, moreWeights, index)
	}
	index -= moreW
	if index < eqW {
		return pivot
	}
	index -= eqW
	if index < lessW {
		return WeightedQuickSelect(less, lessWeights, index)
	}
	return big.Zero()
}
