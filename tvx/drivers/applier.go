package drivers

import (
	"context"
	"errors"

	acrypto "github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/puppet"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	vtypes "github.com/filecoin-project/oni/tvx/chain/types"
)

// Applier applies messages to state trees and storage.
type Applier struct {
	stateWrapper *StateWrapper
	syscalls     vm.SyscallBuilder
}

func NewApplier(sw *StateWrapper, syscalls vm.SyscallBuilder) *Applier {
	return &Applier{sw, syscalls}
}

func (a *Applier) ApplyMessage(epoch abi.ChainEpoch, lm *types.Message) (vtypes.ApplyMessageResult, error) {
	receipt, penalty, reward, err := a.applyMessage(epoch, lm)

	return vtypes.ApplyMessageResult{
		Receipt: receipt,
		Penalty: penalty,
		Reward:  reward,
		Root:    a.stateWrapper.Root().String(),
	}, err
}

func (a *Applier) ApplySignedMessage(epoch abi.ChainEpoch, msg *types.SignedMessage) (vtypes.ApplyMessageResult, error) {
	var lm types.ChainMsg
	switch msg.Signature.Type {
	case acrypto.SigTypeSecp256k1:
		lm = msg
	case acrypto.SigTypeBLS:
		lm = &msg.Message
	default:
		return vtypes.ApplyMessageResult{}, errors.New("Unknown signature type")
	}
	// TODO: Validate the sig first
	receipt, penalty, reward, err := a.applyMessage(epoch, lm)
	return vtypes.ApplyMessageResult{
		Receipt: receipt,
		Penalty: penalty,
		Reward:  reward,
		Root:    a.stateWrapper.Root().String(),
	}, err

}

func (a *Applier) ApplyTipSetMessages(epoch abi.ChainEpoch, blocks []vtypes.BlockMessagesInfo, rnd RandomnessSource) (vtypes.ApplyTipSetResult, error) {
	cs := store.NewChainStore(a.stateWrapper.bs, a.stateWrapper.ds, a.syscalls)
	sm := stmgr.NewStateManager(cs)

	var bms []stmgr.BlockMessages
	for _, b := range blocks {
		bm := stmgr.BlockMessages{
			Miner:    b.Miner,
			WinCount: 1,
		}

		for _, m := range b.BLSMessages {
			bm.BlsMessages = append(bm.BlsMessages, m)
		}

		for _, m := range b.SECPMessages {
			bm.SecpkMessages = append(bm.SecpkMessages, m)
		}

		bms = append(bms, bm)
	}

	var receipts []vtypes.MessageReceipt
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
	})
	if err != nil {
		return vtypes.ApplyTipSetResult{}, err
	}

	a.stateWrapper.stateRoot = sroot

	return vtypes.ApplyTipSetResult{
		Receipts: receipts,
		Root:     a.stateWrapper.Root().String(),
	}, nil
}

func (a *Applier) applyMessage(epoch abi.ChainEpoch, lm types.ChainMsg) (vtypes.MessageReceipt, abi.TokenAmount, abi.TokenAmount, error) {
	ctx := context.TODO()
	base := a.stateWrapper.Root()

	lotusVM, err := vm.NewVM(base, epoch, &vmRand{}, a.stateWrapper.bs, a.syscalls, nil)
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
