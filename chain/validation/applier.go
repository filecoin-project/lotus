package validation

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"

	vtypes "github.com/filecoin-project/chain-validation/chain/types"
	vdrivers "github.com/filecoin-project/chain-validation/drivers"
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

func (a *Applier) ApplyMessage(eCtx *vtypes.ExecutionContext, state vstate.VMWrapper, message *vtypes.Message) (vtypes.MessageReceipt, abi.TokenAmount, abi.TokenAmount, error) {
	lm := toLotusMsg(message)
	return a.applyMessage(eCtx, state, lm)
}

func (a *Applier) ApplyTipSetMessages(state vstate.VMWrapper, blocks []vtypes.BlockMessagesInfo, epoch abi.ChainEpoch, rnd vstate.RandomnessSource) ([]vtypes.MessageReceipt, error) {
	sw := state.(*StateWrapper)
	cs := store.NewChainStore(sw.bs, sw.ds, vdrivers.NewChainValidationSyscalls())
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
			bm.SecpkMessages = append(bm.SecpkMessages, toLotusSignedMsg(m))
		}

		bms = append(bms, bm)
	}

	var receipts []vtypes.MessageReceipt
	sroot, _, err := sm.ApplyBlocks(context.TODO(), state.Root(), bms, epoch, &randWrapper{rnd}, func(c cid.Cid, msg *types.Message, ret *vm.ApplyRet) error {
		if msg.From == builtin.SystemActorAddr {
			return nil // ignore reward and cron calls
		}
		rval := ret.Return
		if rval == nil {
			rval = []byte{} // chain validation tests expect empty arrays to not be nil...
		}
		receipts = append(receipts, vtypes.MessageReceipt{
			ExitCode:    ret.ExitCode,
			ReturnValue: rval,

			GasUsed: vtypes.GasUnits(ret.GasUsed),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	state.(*StateWrapper).stateRoot = sroot

	return receipts, nil
}
func (a *Applier) ApplySignedMessage(eCtx *vtypes.ExecutionContext, state vstate.VMWrapper, msg *vtypes.SignedMessage) (vtypes.MessageReceipt, abi.TokenAmount, abi.TokenAmount, error) {
	var lm types.ChainMsg
	switch msg.Signature.Type {
	case crypto.SigTypeSecp256k1:
		lm = toLotusSignedMsg(msg)
	case crypto.SigTypeBLS:
		lm = toLotusMsg(&msg.Message)
	default:
		return vtypes.MessageReceipt{}, big.Zero(), big.Zero(), xerrors.New("Unknown signature type")
	}
	// TODO: Validate the sig first
	return a.applyMessage(eCtx, state, lm)
}

type randWrapper struct {
	rnd vstate.RandomnessSource
}

func (w *randWrapper) GetRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return w.rnd.Randomness(ctx, pers, round, entropy)
}

type vmRand struct {
	eCtx *vtypes.ExecutionContext
}

func (*vmRand) GetRandomness(ctx context.Context, dst crypto.DomainSeparationTag, h abi.ChainEpoch, input []byte) ([]byte, error) {
	panic("implement me")
}

func (a *Applier) applyMessage(eCtx *vtypes.ExecutionContext, state vstate.VMWrapper, lm types.ChainMsg) (vtypes.MessageReceipt, abi.TokenAmount, abi.TokenAmount, error) {
	ctx := context.TODO()
	st := state.(*StateWrapper)

	base := st.Root()
	randSrc := &vmRand{eCtx}
	lotusVM, err := vm.NewVM(base, eCtx.Epoch, randSrc, st.bs, vdrivers.NewChainValidationSyscalls())
	if err != nil {
		return vtypes.MessageReceipt{}, big.Zero(), big.Zero(), err
	}

	ret, err := lotusVM.ApplyMessage(ctx, lm)
	if err != nil {
		return vtypes.MessageReceipt{}, big.Zero(), big.Zero(), err
	}

	rval := ret.Return
	if rval == nil {
		rval = []byte{}
	}

	st.stateRoot, err = lotusVM.Flush(ctx)
	if err != nil {
		return vtypes.MessageReceipt{}, big.Zero(), big.Zero(), err
	}

	mr := vtypes.MessageReceipt{
		ExitCode:    ret.ExitCode,
		ReturnValue: rval,
		GasUsed:     vtypes.GasUnits(ret.GasUsed),
	}

	return mr, ret.Penalty, abi.NewTokenAmount(ret.GasUsed), nil
}

func toLotusMsg(msg *vtypes.Message) *types.Message {
	return &types.Message{
		To:   msg.To,
		From: msg.From,

		Nonce:  msg.CallSeqNum,
		Method: msg.Method,

		Value:    types.BigInt{Int: msg.Value.Int},
		GasPrice: types.BigInt{Int: msg.GasPrice.Int},
		GasLimit: msg.GasLimit,

		Params: msg.Params,
	}
}

func toLotusSignedMsg(msg *vtypes.SignedMessage) *types.SignedMessage {
	return &types.SignedMessage{
		Message:   *toLotusMsg(&msg.Message),
		Signature: msg.Signature,
	}
}
