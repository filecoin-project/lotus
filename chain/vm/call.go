package vm

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	cid "github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

func CallRaw(ctx context.Context, cs *store.ChainStore, msg *types.Message, bstate cid.Cid, bheight uint64) (*types.MessageReceipt, error) {
	vmi, err := NewVM(bstate, bheight, actors.NetworkAddress, cs)
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

	fromActor, err := vmi.cstate.GetActor(msg.From)
	if err != nil {
		return nil, err
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

func Call(ctx context.Context, cs *store.ChainStore, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	if ts == nil {
		ts = cs.GetHeaviestTipSet()
	}
	state, err := cs.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	return CallRaw(ctx, cs, msg, state, ts.Height())
}
