package chain

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"golang.org/x/xerrors"
)

func Call(ctx context.Context, cs *store.ChainStore, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	if ts == nil {
		ts = cs.GetHeaviestTipSet()
	}
	state, err := cs.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	vmi, err := vm.NewVM(state, ts.Height(), ts.Blocks()[0].Miner, cs)
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
	if msg.Params == nil {
		msg.Params, err = actors.SerializeParams(struct{}{})
		if err != nil {
			return nil, err
		}
	}

	// TODO: maybe just use the invoker directly?
	ret, err := vmi.ApplyMessage(ctx, msg)
	if ret.ActorErr != nil {
		log.Warnf("chain call failed: %s", ret.ActorErr)
	}
	return &ret.MessageReceipt, err
}
