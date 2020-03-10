package validation

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-sectorbuilder"

	vtypes "github.com/filecoin-project/chain-validation/chain/types"
	vstate "github.com/filecoin-project/chain-validation/state"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

// Applier applies messages to state trees and storage.
type Applier struct {
}

var _ vstate.Applier = &Applier{}

func NewApplier() *Applier {
	return &Applier{}
}

func (a *Applier) ApplyMessage(eCtx *vtypes.ExecutionContext, state vstate.VMWrapper, message *vtypes.Message) (vtypes.MessageReceipt, error) {
	ctx := context.TODO()
	st := state.(*StateWrapper)

	base := st.Root()
	randSrc := &vmRand{eCtx}
	lotusVM, err := vm.NewVM(base, abi.ChainEpoch(eCtx.Epoch), randSrc, eCtx.Miner, st.bs, vm.Syscalls(sectorbuilder.ProofVerifier))
	if err != nil {
		return vtypes.MessageReceipt{}, err
	}

	ret, err := lotusVM.ApplyMessage(ctx, toLotusMsg(message))
	if err != nil {
		return vtypes.MessageReceipt{}, err
	}

	st.stateRoot, err = lotusVM.Flush(ctx)
	if err != nil {
		return vtypes.MessageReceipt{}, err
	}

	mr := vtypes.MessageReceipt{
		ExitCode:    exitcode.ExitCode(ret.ExitCode),
		ReturnValue: ret.Return,
		GasUsed:     ret.GasUsed,
	}

	return mr, nil
}

func (a *Applier) ApplyTipSetMessages(state vstate.VMWrapper, blocks []vtypes.BlockMessagesInfo, epoch abi.ChainEpoch, rnd vstate.RandomnessSource) ([]vtypes.MessageReceipt, error) {
	sw := state.(*StateWrapper)
	cs := store.NewChainStore(sw.bs, sw.ds, nil)
	sm := stmgr.NewStateManager(cs)

	var bms []stmgr.BlockMessages
	for _, b := range blocks {
		bm := stmgr.BlockMessages{
			Miner:       b.Miner,
			TicketCount: 1,
		}

		for _, m := range b.BLSMessages {
			bm.BlsMessages = append(bm.BlsMessages, toLotusMsg(m))
		}

		for _, m := range b.SECPMessages {
			bm.SecpkMessages = append(bm.SecpkMessages, toLotusMsg(&m.Message))
		}

		bms = append(bms, bm)
	}

	var receipts []vtypes.MessageReceipt
	sroot, _, err := sm.ApplyBlocks(context.TODO(), state.Root(), bms, epoch, &randWrapper{rnd}, func(c cid.Cid, msg *types.Message, ret *vm.ApplyRet) error {
		if msg.From == builtin.SystemActorAddr {
			return nil // ignore reward and cron calls
		}
		receipts = append(receipts, vtypes.MessageReceipt{
			ExitCode:    exitcode.ExitCode(ret.ExitCode),
			ReturnValue: ret.Return,

			GasUsed: ret.GasUsed,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	state.(*StateWrapper).stateRoot = sroot

	return receipts, nil
}

type randWrapper struct {
	rnd vstate.RandomnessSource
}

func (w *randWrapper) GetRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round int64, entropy []byte) ([]byte, error) {
	return w.rnd.Randomness(ctx, pers, abi.ChainEpoch(round), entropy)
}

type vmRand struct {
	eCtx *vtypes.ExecutionContext
}

func (*vmRand) GetRandomness(ctx context.Context, dst crypto.DomainSeparationTag, h int64, input []byte) ([]byte, error) {
	panic("implement me")
}

func toLotusMsg(msg *vtypes.Message) *types.Message {
	return &types.Message{
		To:   msg.To,
		From: msg.From,

		Nonce:  uint64(msg.CallSeqNum),
		Method: msg.Method,

		Value:    types.BigInt{Int: msg.Value.Int},
		GasPrice: types.BigInt{Int: msg.GasPrice.Int},
		GasLimit: types.NewInt(uint64(msg.GasLimit)),

		Params: msg.Params,
	}
}
