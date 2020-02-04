package actors

import (
	"github.com/filecoin-project/lotus/chain/types"

	samsig "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
)

type MultiSigActor struct{}
type MultiSigActorState = samsig.MultiSigActorState
type MultiSigTransaction = samsig.MultiSigTransaction
type TxnID = samsig.TxnID

type musigMethods struct {
	MultiSigConstructor uint64
	Propose             uint64
	Approve             uint64
	Cancel              uint64
	ClearCompleted      uint64
	AddSigner           uint64
	RemoveSigner        uint64
	SwapSigner          uint64
	ChangeRequirement   uint64
}

var MultiSigMethods = musigMethods{1, 2, 3, 4, 5, 6, 7, 8, 9}

func (msa MultiSigActor) Exports() []interface{} {
	return []interface{}{
		1: msa.MultiSigConstructor,
		2: msa.Propose,
		3: msa.Approve,
		4: msa.Cancel,
		//5: msa.ClearCompleted,
		6: msa.AddSigner,
		7: msa.RemoveSigner,
		8: msa.SwapSigner,
		9: msa.ChangeRequirement,
	}
}

type MultiSigConstructorParams = samsig.ConstructorParams

func (MultiSigActor) MultiSigConstructor(act *types.Actor, vmctx types.VMContext,
	params *MultiSigConstructorParams) ([]byte, ActorError) {

	shim := &runtimeShim{vmctx: vmctx}
	return shim.shimCall(func() interface{} {
		return (&samsig.MultiSigActor{}).Constructor(shim, params)
	})
}

type MultiSigProposeParams = samsig.ProposeParams

func (msa MultiSigActor) Propose(act *types.Actor, vmctx types.VMContext,
	params *MultiSigProposeParams) ([]byte, ActorError) {

	shim := &runtimeShim{vmctx: vmctx}
	return shim.shimCall(func() interface{} {
		return (&samsig.MultiSigActor{}).Propose(shim, params)
	})
}

type MultiSigTxID = samsig.TxnIDParams

func (msa MultiSigActor) Approve(act *types.Actor, vmctx types.VMContext,
	params *MultiSigTxID) ([]byte, ActorError) {

	shim := &runtimeShim{vmctx: vmctx}
	return shim.shimCall(func() interface{} {
		return (&samsig.MultiSigActor{}).Approve(shim, params)
	})
}

func (msa MultiSigActor) Cancel(act *types.Actor, vmctx types.VMContext,
	params *MultiSigTxID) ([]byte, ActorError) {

	shim := &runtimeShim{vmctx: vmctx}
	return shim.shimCall(func() interface{} {
		return (&samsig.MultiSigActor{}).Cancel(shim, params)
	})
}

type MultiSigAddSignerParam = samsig.AddSigner

func (msa MultiSigActor) AddSigner(act *types.Actor, vmctx types.VMContext,
	params *MultiSigAddSignerParam) ([]byte, ActorError) {

	shim := &runtimeShim{vmctx: vmctx}
	return shim.shimCall(func() interface{} {
		return (&samsig.MultiSigActor{}).AddSigner(shim, params)
	})
}

type MultiSigRemoveSignerParam = samsig.RemoveSigner

func (msa MultiSigActor) RemoveSigner(act *types.Actor, vmctx types.VMContext,
	params *MultiSigRemoveSignerParam) ([]byte, ActorError) {

	shim := &runtimeShim{vmctx: vmctx}
	return shim.shimCall(func() interface{} {
		return (&samsig.MultiSigActor{}).RemoveSigner(shim, params)
	})
}

type MultiSigSwapSignerParams = samsig.SwapSignerParams

func (msa MultiSigActor) SwapSigner(act *types.Actor, vmctx types.VMContext,
	params *MultiSigSwapSignerParams) ([]byte, ActorError) {

	shim := &runtimeShim{vmctx: vmctx}
	return shim.shimCall(func() interface{} {
		return (&samsig.MultiSigActor{}).SwapSigner(shim, params)
	})
}

type MultiSigChangeReqParams = samsig.ChangeNumApprovalsThresholdParams

func (msa MultiSigActor) ChangeRequirement(act *types.Actor, vmctx types.VMContext,
	params *MultiSigChangeReqParams) ([]byte, ActorError) {

	shim := &runtimeShim{vmctx: vmctx}
	return shim.shimCall(func() interface{} {
		return (&samsig.MultiSigActor{}).ChangeNumApprovalsThreshold(shim, params)
	})
}
