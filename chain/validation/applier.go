package validation

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/filecoin-project/go-sectorbuilder"

	vtypes "github.com/filecoin-project/chain-validation/chain/types"
	vstate "github.com/filecoin-project/chain-validation/state"

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

	return mr, ret.ActorErr
}

func (a *Applier) ApplyTipSetMessages(state vstate.VMWrapper, blocks []vtypes.BlockMessagesInfo, epoch abi.ChainEpoch, rnd vstate.RandomnessSource) ([]vtypes.MessageReceipt, error) {
	panic("implement me")
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

		Value:    types.BigInt{msg.Value.Int},
		GasPrice: types.BigInt{msg.GasPrice.Int},
		GasLimit: types.NewInt(uint64(msg.GasLimit)),

		Params: msg.Params,
	}
}
