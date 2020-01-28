package actors

import (
	"bytes"
	"context"
	"fmt"
	"runtime/debug"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/specs-actors/actors/abi"
	samsig "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	cbg "github.com/whyrusleeping/cbor-gen"
)

type MultiSigActor struct{}
type MultiSigActorState = samsig.MultiSigActorState

/*
func (msas MultiSigActorState) canSpend(act *types.Actor, amnt types.BigInt, height uint64) bool {
	if msas.UnlockDuration == 0 {
		return true
	}

	offset := height - msas.StartingBlock
	if offset > msas.UnlockDuration {
		return true
	}

	minBalance := types.BigDiv(msas.InitialBalance, types.NewInt(msas.UnlockDuration))
	minBalance = types.BigMul(minBalance, types.NewInt(offset))
	return !minBalance.LessThan(types.BigSub(act.Balance, amnt))
}

func (msas MultiSigActorState) isSigner(addr address.Address) bool {
	for _, s := range msas.Signers {
		if s == addr {
			return true
		}
	}
	return false
}

func (msas MultiSigActorState) getTransaction(txid uint64) (*MTransaction, ActorError) {
	if txid >= uint64(len(msas.Transactions)) {
		return nil, aerrors.Newf(1, "could not get transaction (numbers of tx %d,want to get txid %d)", len(msas.Transactions), txid)
	}
	return &msas.Transactions[txid], nil
}
*/

type MTransaction struct {
	Created uint64 // NOT USED ??
	TxID    uint64

	To     address.Address
	Value  types.BigInt
	Method uint64
	Params []byte

	Approved []address.Address
	Complete bool
	Canceled bool
	RetCode  uint64
}

func (tx MTransaction) Active() ActorError {
	if tx.Complete {
		return aerrors.New(2, "transaction already completed")
	}
	if tx.Canceled {
		return aerrors.New(3, "transaction canceled")
	}
	return nil
}

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

type runtimeShim struct {
	vmctx types.VMContext
	vmr.Runtime
}

func (rs *runtimeShim) shimCall(f func() interface{}) (rval []byte, aerr ActorError) {
	defer func() {
		if r := recover(); r != nil {
			if ar, ok := r.(ActorError); ok {
				aerr = ar
				return
			}
			fmt.Println("caught one of those actor errors: ", r)
			debug.PrintStack()
			log.Errorf("ERROR")
			aerr = aerrors.Newf(1, "generic spec actors failure")
		}
	}()

	ret := f()
	switch ret := ret.(type) {
	case []byte:
		return ret, nil
	case cbg.CBORMarshaler:
		buf := new(bytes.Buffer)
		if err := ret.MarshalCBOR(buf); err != nil {
			return nil, aerrors.Absorb(err, 2, "failed to marshal response to cbor")
		}
		return buf.Bytes(), nil
	case nil:
		return nil, nil
	default:
		return nil, aerrors.New(3, "could not determine type for response from call")
	}
}

func (rs *runtimeShim) ValidateImmediateCallerIs(as ...address.Address) {
	for _, a := range as {
		if rs.vmctx.Message().From == a {
			return
		}
	}
	fmt.Println("Caller: ", rs.vmctx.Message().From, as)
	panic("we like to panic when people call the wrong methods")
}

func (rs *runtimeShim) ImmediateCaller() address.Address {
	return rs.vmctx.Message().From
}

func (rs *runtimeShim) Context() context.Context {
	return rs.vmctx.Context()
}

func (rs *runtimeShim) IpldGet(c cid.Cid, o vmr.CBORUnmarshaler) bool {
	if err := rs.vmctx.Storage().Get(c, o); err != nil {
		panic(err) // y o o o o o l l l l o o o o o
	}
	return true
}

func (rs *runtimeShim) IpldPut(o vmr.CBORMarshaler) cid.Cid {
	c, err := rs.vmctx.Storage().Put(o)
	if err != nil {
		panic(err)
	}
	return c
}

func (rs *runtimeShim) Abort(code exitcode.ExitCode, msg string, args ...interface{}) {
	panic(aerrors.Newf(uint8(code), msg, args...))
}

func (rs *runtimeShim) AbortStateMsg(msg string) {
	rs.Abort(101, msg)
}

func (rs *runtimeShim) ValidateImmediateCallerType(...cid.Cid) {
	log.Info("validate caller type is dumb")
}

func (rs *runtimeShim) CurrentBalance() abi.TokenAmount {
	b, err := rs.vmctx.GetBalance(rs.vmctx.Message().From)
	if err != nil {
		rs.Abort(1, err.Error())
	}

	return abi.TokenAmount(b)
}

func (rs *runtimeShim) CurrEpoch() abi.ChainEpoch {
	return abi.ChainEpoch(rs.vmctx.BlockHeight())
}

type dumbWrapperType struct {
	val []byte
}

func (dwt *dumbWrapperType) Into(um vmr.CBORUnmarshaler) error {
	return um.UnmarshalCBOR(bytes.NewReader(dwt.val))
}

func (rs *runtimeShim) Send(to address.Address, method abi.MethodNum, params abi.MethodParams, value abi.TokenAmount) (vmr.SendReturn, exitcode.ExitCode) {
	ret, err := rs.vmctx.Send(to, uint64(method), types.BigInt(value), []byte(params))
	if err != nil {
		if err.IsFatal() {
			panic(err)
		}
		return nil, exitcode.ExitCode(err.RetCode())
	}
	return &dumbWrapperType{ret}, 0
}

func (rs *runtimeShim) State() vmr.StateHandle {
	return &shimStateHandle{rs: rs}
}

type shimStateHandle struct {
	rs *runtimeShim
}

func (ssh *shimStateHandle) Readonly(obj vmr.CBORUnmarshaler) {
	if err := ssh.rs.vmctx.Storage().Get(ssh.rs.vmctx.Storage().GetHead(), obj); err != nil {
		panic(err)
	}
}

func (ssh *shimStateHandle) Transaction(obj vmr.CBORer, f func() interface{}) interface{} {
	head := ssh.rs.vmctx.Storage().GetHead()
	if err := ssh.rs.vmctx.Storage().Get(head, obj); err != nil {
		panic(err)
	}

	out := f()

	c, err := ssh.rs.vmctx.Storage().Put(obj)
	if err != nil {
		panic(err)
	}
	if err := ssh.rs.vmctx.Storage().Commit(head, c); err != nil {
		panic(err)
	}

	return out
}

func (ssh *shimStateHandle) Construct(f func() vmr.CBORMarshaler) {
	out := f()

	c, err := ssh.rs.vmctx.Storage().Put(out)
	if err != nil {
		panic(err)
	}
	if err := ssh.rs.vmctx.Storage().Commit(EmptyCBOR, c); err != nil {
		panic(err)
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

/*
func (MultiSigActor) load(vmctx types.VMContext) (cid.Cid, *MultiSigActorState, ActorError) {
	var self MultiSigActorState
	head := vmctx.Storage().GetHead()

	err := vmctx.Storage().Get(head, &self)
	if err != nil {
		return cid.Undef, nil, aerrors.Wrap(err, "could not get self")
	}
	return head, &self, nil
}

func (msa MultiSigActor) loadAndVerify(vmctx types.VMContext) (cid.Cid, *MultiSigActorState, ActorError) {
	head, self, err := msa.load(vmctx)
	if err != nil {
		return cid.Undef, nil, err
	}

	if !self.isSigner(vmctx.Message().From) {
		return cid.Undef, nil, aerrors.New(1, "not authorized")
	}
	return head, self, nil
}

func (MultiSigActor) save(vmctx types.VMContext, oldHead cid.Cid, self *MultiSigActorState) ActorError {
	newHead, err := vmctx.Storage().Put(self)
	if err != nil {
		return aerrors.Wrap(err, "could not put new head")
	}
	err = vmctx.Storage().Commit(oldHead, newHead)
	if err != nil {
		return aerrors.Wrap(err, "could not commit new head")
	}
	return nil

}
*/

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
