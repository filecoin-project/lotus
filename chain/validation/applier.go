package validation

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/puppet"
	"github.com/ipfs/go-cid"

	vtypes "github.com/filecoin-project/chain-validation/chain/types"
	vstate "github.com/filecoin-project/chain-validation/state"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

// Applier applies messages to state trees and storage.
type Applier struct {
	stateWrapper *StateWrapper
	syscalls     vm.SyscallBuilder
}

var _ vstate.Applier = &Applier{}

func NewApplier(sw *StateWrapper, syscalls vm.SyscallBuilder) *Applier {
	return &Applier{sw, syscalls}
}

func (a *Applier) ApplyMessage(epoch abi.ChainEpoch, message *vtypes.Message) (vtypes.ApplyMessageResult, error) {
	lm := toLotusMsg(message)
	receipt, penalty, reward, err := a.applyMessage(epoch, lm)
	return vtypes.ApplyMessageResult{
		Msg:     *message,
		Receipt: receipt,
		Penalty: penalty,
		Reward:  reward,
		Root:    a.stateWrapper.Root().String(),
	}, err
}

func (a *Applier) ApplySignedMessage(epoch abi.ChainEpoch, msg *vtypes.SignedMessage) (vtypes.ApplyMessageResult, error) {
	var lm types.ChainMsg
	switch msg.Signature.Type {
	case crypto.SigTypeSecp256k1:
		lm = toLotusSignedMsg(msg)
	case crypto.SigTypeBLS:
		lm = toLotusMsg(&msg.Message)
	default:
		return vtypes.ApplyMessageResult{}, xerrors.New("Unknown signature type")
	}
	// TODO: Validate the sig first
	receipt, penalty, reward, err := a.applyMessage(epoch, lm)
	return vtypes.ApplyMessageResult{
		Msg:     msg.Message,
		Receipt: receipt,
		Penalty: penalty,
		Reward:  reward,
		Root:    a.stateWrapper.Root().String(),
	}, err

}

func (a *Applier) ApplyTipSetMessages(epoch abi.ChainEpoch, blocks []vtypes.BlockMessagesInfo, rnd vstate.RandomnessSource) (vtypes.ApplyTipSetResult, error) {
	cs := store.NewChainStore(a.stateWrapper.bs, a.stateWrapper.ds, a.syscalls)
	sm := stmgr.NewStateManager(cs)

	var bms []store.BlockMessages
	for _, b := range blocks {
		bm := store.BlockMessages{
			Miner:    b.Miner,
			WinCount: 1,
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
	// TODO: base fee
	sroot, _, err := sm.ApplyBlocks(context.TODO(), epoch-1, a.stateWrapper.Root(), bms, epoch, &randWrapper{rnd}, func(c cid.Cid, msg *types.Message, ret *vm.ApplyRet) error {
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
	}, abi.NewTokenAmount(100))
	if err != nil {
		return vtypes.ApplyTipSetResult{}, err
	}

	a.stateWrapper.stateRoot = sroot

	return vtypes.ApplyTipSetResult{
		Receipts: receipts,
		Root:     a.stateWrapper.Root().String(),
	}, nil
}

type randWrapper struct {
	rand vstate.RandomnessSource
}

// TODO: these should really be two different randomness sources
func (w *randWrapper) GetChainRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return w.rand.Randomness(ctx, pers, round, entropy)
}

func (w *randWrapper) GetBeaconRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return w.rand.Randomness(ctx, pers, round, entropy)
}

type vmRand struct {
}

func (*vmRand) GetChainRandomness(ctx context.Context, dst crypto.DomainSeparationTag, h abi.ChainEpoch, input []byte) ([]byte, error) {
	panic("implement me")
}

func (*vmRand) GetBeaconRandomness(ctx context.Context, dst crypto.DomainSeparationTag, h abi.ChainEpoch, input []byte) ([]byte, error) {
	panic("implement me")
}

func (a *Applier) applyMessage(epoch abi.ChainEpoch, lm types.ChainMsg) (vtypes.MessageReceipt, abi.TokenAmount, abi.TokenAmount, error) {
	ctx := context.TODO()
	base := a.stateWrapper.Root()

	vmopt := &vm.VMOpts{
		StateBase:      base,
		Epoch:          epoch,
		Rand:           &vmRand{},
		Bstore:         a.stateWrapper.bs,
		Syscalls:       a.syscalls,
		CircSupplyCalc: nil,
		BaseFee:        abi.NewTokenAmount(100),
	}

	lotusVM, err := vm.NewVM(vmopt)
	// need to modify the VM invoker to add the puppet actor
	chainValInvoker := vm.NewInvoker()
	chainValInvoker.Register(puppet.PuppetActorCodeID, puppet.Actor{}, puppet.State{})
	lotusVM.SetInvoker(chainValInvoker)
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

	a.stateWrapper.stateRoot, err = lotusVM.Flush(ctx)
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

		Value:      msg.Value,
		GasLimit:   msg.GasLimit,
		GasFeeCap:  msg.GasFeeCap,
		GasPremium: msg.GasPremium,

		Params: msg.Params,
	}
}

func toLotusSignedMsg(msg *vtypes.SignedMessage) *types.SignedMessage {
	return &types.SignedMessage{
		Message:   *toLotusMsg(&msg.Message),
		Signature: msg.Signature,
	}
}
