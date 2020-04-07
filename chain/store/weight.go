package store

import (
	"context"
	"math/big"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	big2 "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"
)

var zero = types.NewInt(0)

func (cs *ChainStore) Weight(ctx context.Context, ts *types.TipSet) (types.BigInt, error) {
	if ts == nil {
		return types.NewInt(0), nil
	}
	// >>> w[r] <<< + wFunction(totalPowerAtTipset(ts)) * 2^8 + (wFunction(totalPowerAtTipset(ts)) * len(ts.blocks) * wRatio_num * 2^8) / (e * wRatio_den)

	var out = new(big.Int).Set(ts.Blocks()[0].ParentWeight.Int)

	// >>> wFunction(totalPowerAtTipset(ts)) * 2^8 <<< + (wFunction(totalPowerAtTipset(ts)) * len(ts.blocks) * wRatio_num * 2^8) / (e * wRatio_den)

	tpow := big2.Zero()
	{
		cst := cbor.NewCborStore(cs.Blockstore())
		state, err := state.LoadStateTree(cst, ts.ParentState())
		if err != nil {
			return types.NewInt(0), xerrors.Errorf("load state tree: %w", err)
		}

		act, err := state.GetActor(builtin.StoragePowerActorAddr)
		if err != nil {
			return types.NewInt(0), xerrors.Errorf("get power actor: %w", err)
		}

		var st power.State
		if err := cst.Get(ctx, act.Head, &st); err != nil {
			return types.NewInt(0), xerrors.Errorf("get power actor head: %w", err)
		}
		tpow = st.TotalNetworkPower
	}

	log2P := int64(0)
	if tpow.GreaterThan(zero) {
		log2P = int64(tpow.BitLen() - 1)
	} else {
		// Not really expect to be here ...
		return types.EmptyInt, xerrors.Errorf("All power in the net is gone. You network might be disconnected, or the net is dead!")
	}

	out.Add(out, big.NewInt(log2P<<8))

	// (wFunction(totalPowerAtTipset(ts)) * len(ts.blocks) * wRatio_num * 2^8) / (e * wRatio_den)

	eWeight := big.NewInt((log2P * int64(len(ts.Blocks())) * build.WRatioNum) << 8)
	eWeight.Div(eWeight, big.NewInt(int64(build.BlocksPerEpoch*build.WRatioDen)))
	out.Add(out, eWeight)

	return types.BigInt{Int: out}, nil
}

// todo: dedupe with state manager
func (cs *ChainStore) call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	bstate := ts.ParentState()

	r := NewChainRand(cs, ts.Cids(), ts.Height())

	vmi, err := vm.NewVM(bstate, ts.Height(), r, cs.bs, cs.vmcalls)
	if err != nil {
		return nil, xerrors.Errorf("failed to set up vm: %w", err)
	}

	if msg.GasLimit == 0 {
		msg.GasLimit = 10000000000
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
	// TODO: use signed message length for secp messages
	ret, err := vmi.ApplyMessage(ctx, msg)
	if err != nil {
		return nil, xerrors.Errorf("apply message failed: %w", err)
	}

	if ret.ActorErr != nil {
		log.Warnf("chain call failed: %s", ret.ActorErr)
	}
	return &ret.MessageReceipt, nil
}
