package validation

import (
	"context"
	"github.com/filecoin-project/go-sectorbuilder"

	vchain "github.com/filecoin-project/chain-validation/pkg/chain"
	vstate "github.com/filecoin-project/chain-validation/pkg/state"
	vtypes "github.com/filecoin-project/chain-validation/pkg/state/types"

	"github.com/filecoin-project/go-address"
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

func (a *Applier) ApplyMessage(eCtx *vchain.ExecutionContext, state vstate.Wrapper, message interface{}) (vchain.MessageReceipt, error) {
	ctx := context.TODO()
	st := state.(*StateWrapper)

	base := st.Cid()
	randSrc := &vmRand{eCtx}
	minerAddr, err := address.NewFromBytes(eCtx.MinerOwner.Bytes())
	if err != nil {
		return vchain.MessageReceipt{}, err
	}
	lotusVM, err := vm.NewVM(base, eCtx.Epoch, randSrc, minerAddr, st.bs, vm.Syscalls(sectorbuilder.ProofVerifier))
	if err != nil {
		return vchain.MessageReceipt{}, err
	}

	ret, err := lotusVM.ApplyMessage(ctx, message.(*types.Message))
	if err != nil {
		return vchain.MessageReceipt{}, err
	}

	st.stateRoot, err = lotusVM.Flush(ctx)
	if err != nil {
		return vchain.MessageReceipt{}, err
	}

	mr := vchain.MessageReceipt{
		ExitCode:    ret.ExitCode,
		ReturnValue: ret.Return,
		GasUsed:     vtypes.GasUnit(ret.GasUsed.Uint64()),
	}

	return mr, ret.ActorErr
}

type vmRand struct {
	eCtx *vchain.ExecutionContext
}

func (*vmRand) GetRandomness(ctx context.Context, h int64) ([]byte, error) {
	panic("implement me")
}
