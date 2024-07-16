package store

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

func ComputeNextBaseFee(baseFee types.BigInt, gasLimitUsed int64, noOfBlocks int, epoch abi.ChainEpoch) types.BigInt {
	// deta := gasLimitUsed/noOfBlocks - buildconstants.BlockGasTarget
	// change := baseFee * deta / BlockGasTarget
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

	zero := abi.NewTokenAmount(0)

	// totalLimit is sum of GasLimits of unique messages in a tipset
	totalLimit := int64(0)

	seen := make(map[cid.Cid]struct{})

	for _, b := range ts.Blocks() {
		msg1, msg2, err := cs.MessagesForBlock(ctx, b)
		if err != nil {
			return zero, xerrors.Errorf("error getting messages for: %s: %w", b.Cid(), err)
		}
		for _, m := range msg1 {
			c := m.Cid()
			if _, ok := seen[c]; !ok {
				totalLimit += m.GasLimit
				seen[c] = struct{}{}
			}
		}
		for _, m := range msg2 {
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
