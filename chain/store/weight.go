package store

import (
	"context"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"golang.org/x/xerrors"
)

func (cs *ChainStore) Weight(ctx context.Context, ts *types.TipSet) (types.BigInt, error) {
	if ts == nil {
		return types.NewInt(0), nil
	}
	// w[r+1] = w[r] + wFunction(totalPowerAtTipset(ts)) * 2^8 + (wFunction(totalPowerAtTipset(ts)) * len(ts.blocks) * wRatio_num * 2^8) / (e * wRatio_den)

	// wr = wRatio_num(0.5) * 2^8 / wRatio_den(2)
	wr := types.NewInt(256)

	// wFunction(totalPowerAtTipset(ts)) * 2^8 + (wFunction(totalPowerAtTipset(ts)) * len(ts.blocks) * wr ) / e

	// /////
	// wFunction(totalPowerAtTipset(ts))

	ret, err := cs.call(ctx, &types.Message{
		From:   actors.StorageMarketAddress,
		To:     actors.StorageMarketAddress,
		Method: actors.SPAMethods.GetTotalStorage,
	}, ts)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to get total power from chain: %w", err)
	}
	if ret.ExitCode != 0 {
		return types.EmptyInt, xerrors.Errorf("failed to get total power from chain (exit code %d)", ret.ExitCode)
	}

	totalPowerAtTipsetL2 := types.BigFromBytes(ret.Return).BitLen()

	// //////
	// w[r] + wFunction(totalPowerAtTipset(ts)) * 2^8

	out := types.BigAdd(ts.Blocks()[0].ParentWeight, types.NewInt(uint64(totalPowerAtTipsetL2*256)))

	// //////
	// (wFunction(totalPowerAtTipset(ts)) * len(ts.blocks) * wr)
	eWeight := types.BigMul(types.BigMul(types.NewInt(uint64(totalPowerAtTipsetL2)), types.NewInt(uint64(len(ts.Blocks())))), wr)
	eWeight = types.BigDiv(eWeight, types.NewInt(build.BlocksPerEpoch))

	return types.BigAdd(out, eWeight), nil
}

// todo: dedupe with state manager
func (cs *ChainStore) call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	bstate := ts.ParentState()

	r := NewChainRand(cs, ts.Cids(), ts.Height(), nil)

	vmi, err := vm.NewVM(bstate, ts.Height(), r, actors.NetworkAddress, cs.bs)
	if err != nil {
		return nil, xerrors.Errorf("failed to set up vm: %w", err)
	}

	if msg.GasLimit == types.EmptyInt {
		msg.GasLimit = types.NewInt(10000000000)
	}
	if msg.GasPrice == types.EmptyInt {
		msg.GasPrice = types.NewInt(0)
	}
	if msg.Value == types.EmptyInt {
		msg.Value = types.NewInt(0)
	}

	fromActor, err := vmi.StateTree().GetActor(msg.From)
	if err != nil {
		return nil, xerrors.Errorf("call raw get actor: %s", err)
	}

	msg.Nonce = fromActor.Nonce

	// TODO: maybe just use the invoker directly?
	ret, err := vmi.ApplyMessage(ctx, msg)
	if err != nil {
		return nil, xerrors.Errorf("apply message failed: %w", err)
	}

	if ret.ActorErr != nil {
		log.Warnf("chain call failed: %s", ret.ActorErr)
	}
	return &ret.MessageReceipt, nil
}
