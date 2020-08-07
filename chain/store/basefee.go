package store

import (
	"context"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"golang.org/x/xerrors"
)

func computeNextBaseFee(baseFee types.BigInt, gasLimitUsed int64, noOfBlocks int) types.BigInt {
	delta := gasLimitUsed/int64(noOfBlocks) - build.BlockGasTarget

	change := big.Mul(baseFee, big.NewInt(delta))
	change = big.Div(change, big.NewInt(build.BlockGasTarget))
	change = big.Div(change, big.NewInt(build.BaseFeeMaxChangeDenom))

	nextBaseFee := big.Add(baseFee, change)
	if big.Cmp(nextBaseFee, big.NewInt(build.MinimumBaseFee)) < 0 {
		nextBaseFee = big.NewInt(build.MinimumBaseFee)
	}
	return nextBaseFee
}

func (cs *ChainStore) ComputeBaseFee(ctx context.Context, ts *types.TipSet) (abi.TokenAmount, error) {
	zero := abi.NewTokenAmount(0)
	totalLimit := int64(0)
	for _, b := range ts.Blocks() {
		msg1, msg2, err := cs.MessagesForBlock(b)
		if err != nil {
			return zero, xerrors.Errorf("error getting messages for: %s: %w", b.Cid(), err)
		}
		for _, m := range msg1 {
			totalLimit += m.GasLimit
		}
		for _, m := range msg2 {
			totalLimit += m.Message.GasLimit
		}
	}
	parentBaseFee := ts.Blocks()[0].ParentBaseFee

	return computeNextBaseFee(parentBaseFee, totalLimit, len(ts.Blocks())), nil
}
