package validation

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	vchain "github.com/filecoin-project/chain-validation/pkg/chain"
	vstate "github.com/filecoin-project/chain-validation/pkg/state"

	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

// Applier applies messages to state trees and storage.
type Applier struct {
}

var _ vchain.Applier = &Applier{}

func NewApplier() *Applier {
	return &Applier{}
}

func (a *Applier) ApplyMessage(eCtx *vchain.ExecutionContext, state vstate.Wrapper, message *vchain.Message) (vchain.MessageReceipt, error) {
	ctx := context.TODO()
	st := state.(*StateWrapper)

	base := st.Cid()
	randSrc := &vmRand{eCtx}
	minerAddr, err := address.NewFromBytes(eCtx.MinerOwner.Bytes())
	if err != nil {
		return vchain.MessageReceipt{}, err
	}
	lotusVM, err := vm.NewVM(base, uint64(eCtx.Epoch), randSrc, minerAddr, st.bs, vm.Syscalls(sectorbuilder.ProofVerifier))
	if err != nil {
		return vchain.MessageReceipt{}, err
	}

	ret, err := lotusVM.ApplyMessage(ctx, toLotusMsg(message))
	if err != nil {
		return vchain.MessageReceipt{}, err
	}

	st.stateRoot, err = lotusVM.Flush(ctx)
	if err != nil {
		return vchain.MessageReceipt{}, err
	}

	mr := vchain.MessageReceipt{
		ExitCode:    exitcode.ExitCode(ret.ExitCode),
		ReturnValue: ret.Return,
		GasUsed:     big.Int{ret.GasUsed.Int},
	}

	return mr, ret.ActorErr
}

type vmRand struct {
	eCtx *vchain.ExecutionContext
}

func (*vmRand) GetRandomness(ctx context.Context, h int64) ([]byte, error) {
	panic("implement me")
}

func toLotusMsg(msg *vchain.Message) *types.Message {
	return &types.Message{
		To:       msg.To,
		From:     msg.From,

		Nonce:    uint64(msg.CallSeqNum),
		Method:   uint64(msg.Method),

		Value:    types.BigInt{msg.Value.Int},
		GasPrice: types.BigInt{msg.GasPrice.Int},
		GasLimit: types.BigInt{msg.GasLimit.Int},

		Params:   msg.Params,
	}
}
