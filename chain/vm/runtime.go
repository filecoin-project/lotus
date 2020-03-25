package vm

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/trace"

	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
)

type Runtime struct {
	ctx context.Context

	vm        *VM
	state     *state.StateTree
	msg       *types.Message
	height    abi.ChainEpoch
	cst       cbor.IpldStore
	pricelist Pricelist

	gasAvailable int64
	gasUsed      int64

	sys runtime.Syscalls

	// address that started invoke chain
	origin      address.Address
	originNonce uint64

	internalExecutions []*ExecutionResult
	numActorsCreated   uint64
}

func (rt *Runtime) ResolveAddress(address address.Address) (ret address.Address, ok bool) {
	r, err := rt.state.LookupID(address)
	if err != nil { // TODO: check notfound
		rt.Abortf(exitcode.ErrPlaceholder, "resolve address: %v", err)
	}
	return r, true
}

func (rs *Runtime) Get(c cid.Cid, o vmr.CBORUnmarshaler) bool {
	if err := rs.cst.Get(context.TODO(), c, o); err != nil {
		// TODO: err not found?
		rs.Abortf(exitcode.ErrPlaceholder, "storage get: %v", err)
	}
	return true
}

func (rs *Runtime) Put(x vmr.CBORMarshaler) cid.Cid {
	c, err := rs.cst.Put(context.TODO(), x)
	if err != nil {
		rs.Abortf(exitcode.ErrPlaceholder, "storage put: %v", err) // todo: spec code?
	}
	return c
}

var _ vmr.Runtime = (*Runtime)(nil)

func (rs *Runtime) shimCall(f func() interface{}) (rval []byte, aerr aerrors.ActorError) {
	defer func() {
		if r := recover(); r != nil {
			if ar, ok := r.(aerrors.ActorError); ok {
				log.Errorf("VM.Call failure: %+v", ar)
				aerr = ar
				return
			}
			log.Errorf("spec actors failure: %s", r)
			aerr = aerrors.Newf(1, "spec actors failure: %s", r)
		}
	}()

	ret := f()
	switch ret := ret.(type) {
	case []byte:
		return ret, nil
	case *adt.EmptyValue:
		return nil, nil
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

func (rs *Runtime) Message() vmr.Message {
	var ok bool

	rawm := *rs.msg
	rawm.From, ok = rs.ResolveAddress(rawm.From)
	if !ok {
		rs.Abortf(exitcode.ErrPlaceholder, "resolve from address failed")
	}

	rawm.To, ok = rs.ResolveAddress(rawm.To)
	if !ok {
		rs.Abortf(exitcode.ErrPlaceholder, "resolve to address failed")
	}

	return &rawm
}

func (rs *Runtime) ValidateImmediateCallerAcceptAny() {
	return
}

func (rs *Runtime) CurrentBalance() abi.TokenAmount {
	b, err := rs.GetBalance(rs.Message().Receiver())
	if err != nil {
		rs.Abortf(exitcode.ExitCode(err.RetCode()), "get current balance: %v", err)
	}
	return b
}

func (rs *Runtime) GetActorCodeCID(addr address.Address) (ret cid.Cid, ok bool) {
	act, err := rs.state.GetActor(addr)
	if err != nil {
		// todo: notfound
		rs.Abortf(exitcode.ErrPlaceholder, "%v", err)
	}

	return act.Code, true
}

func (rt *Runtime) GetRandomness(personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) abi.Randomness {
	res, err := rt.vm.rand.GetRandomness(rt.ctx, personalization, int64(randEpoch), entropy)
	if err != nil {
		rt.Abortf(exitcode.SysErrInternal, "could not get randomness: %s", err)
	}
	return res
}

func (rs *Runtime) Store() vmr.Store {
	return rs
}

func (rt *Runtime) NewActorAddress() address.Address {
	var b bytes.Buffer
	oa, _ := ResolveToKeyAddr(rt.vm.cstate, rt.vm.cst, rt.origin)
	if err := oa.MarshalCBOR(&b); err != nil { // todo: spec says cbor; why not just bytes?
		rt.Abortf(exitcode.ErrSerialization, "writing caller address into a buffer: %v", err)
	}

	if err := binary.Write(&b, binary.BigEndian, rt.originNonce); err != nil {
		rt.Abortf(exitcode.ErrSerialization, "writing nonce address into a buffer: %v", err)
	}
	if err := binary.Write(&b, binary.BigEndian, rt.numActorsCreated); err != nil { // TODO: expose on vm
		rt.Abortf(exitcode.ErrSerialization, "writing callSeqNum address into a buffer: %v", err)
	}
	addr, err := address.NewActorAddress(b.Bytes())
	if err != nil {
		rt.Abortf(exitcode.ErrSerialization, "create actor address: %v", err)
	}

	rt.incrementNumActorsCreated()
	return addr
}

func (rt *Runtime) CreateActor(codeId cid.Cid, address address.Address) {
	rt.ChargeGas(rt.Pricelist().OnCreateActor())
	var err error

	err = rt.state.SetActor(address, &types.Actor{
		Code:    codeId,
		Head:    EmptyObjectCid,
		Nonce:   0,
		Balance: big.Zero(),
	})
	if err != nil {
		rt.Abortf(exitcode.SysErrInternal, "creating actor entry: %v", err)
	}
}

func (rt *Runtime) DeleteActor() {
	rt.ChargeGas(rt.Pricelist().OnDeleteActor())
	act, err := rt.state.GetActor(rt.Message().Receiver())
	if err != nil {
		rt.Abortf(exitcode.SysErrInternal, "failed to load actor in delete actor: %s", err)
	}
	if !act.Balance.IsZero() {
		rt.Abortf(exitcode.SysErrInternal, "cannot delete actor with non-zero balance")
	}
	if err := rt.state.DeleteActor(rt.Message().Receiver()); err != nil {
		rt.Abortf(exitcode.SysErrInternal, "failed to delete actor: %s", err)
	}
}

const GasVerifySignature = 50

func (rs *Runtime) Syscalls() vmr.Syscalls {
	// TODO: Make sure this is wrapped in something that charges gas for each of the calls
	return rs.sys
}

func (rs *Runtime) StartSpan(name string) vmr.TraceSpan {
	panic("implement me")
}

func (rt *Runtime) ValidateImmediateCallerIs(as ...address.Address) {
	imm, ok := rt.ResolveAddress(rt.Message().Caller())
	if !ok {
		rt.Abortf(exitcode.ErrIllegalState, "couldn't resolve immediate caller")
	}

	for _, a := range as {
		if imm == a {
			return
		}
	}
	rt.Abortf(exitcode.SysErrForbidden, "caller %s is not one of %s", rt.Message().Caller(), as)
}

func (rt *Runtime) Context() context.Context {
	return rt.ctx
}

func (rs *Runtime) Abortf(code exitcode.ExitCode, msg string, args ...interface{}) {
	log.Error("Abortf: ", fmt.Sprintf(msg, args...))
	panic(aerrors.NewfSkip(2, uint8(code), msg, args...))
}

func (rs *Runtime) AbortStateMsg(msg string) {
	panic(aerrors.NewfSkip(3, 101, msg))
}

func (rt *Runtime) ValidateImmediateCallerType(ts ...cid.Cid) {
	callerCid, ok := rt.GetActorCodeCID(rt.Message().Caller())
	if !ok {
		rt.Abortf(exitcode.ErrIllegalArgument, "failed to lookup code cid for caller")
	}
	for _, t := range ts {
		if t == callerCid {
			return
		}
	}
	rt.Abortf(exitcode.SysErrForbidden, "caller cid type %q was not one of %v", callerCid, ts)
}

func (rs *Runtime) CurrEpoch() abi.ChainEpoch {
	return rs.height
}

type dumbWrapperType struct {
	val []byte
}

func (dwt *dumbWrapperType) Into(um vmr.CBORUnmarshaler) error {
	return um.UnmarshalCBOR(bytes.NewReader(dwt.val))
}

func (rs *Runtime) Send(to address.Address, method abi.MethodNum, m vmr.CBORMarshaler, value abi.TokenAmount) (vmr.SendReturn, exitcode.ExitCode) {
	var params []byte
	if m != nil {
		buf := new(bytes.Buffer)
		if err := m.MarshalCBOR(buf); err != nil {
			rs.Abortf(exitcode.SysErrInvalidParameters, "failed to marshal input parameters: %s", err)
		}
		params = buf.Bytes()
	}

	ret, err := rs.internalSend(rs.Message().Receiver(), to, method, types.BigInt(value), params)
	if err != nil {
		if err.IsFatal() {
			panic(err)
		}
		log.Warnf("vmctx send failed: to: %s, method: %d: ret: %d, err: %s", to, method, ret, err)
		return nil, exitcode.ExitCode(err.RetCode())
	}
	return &dumbWrapperType{ret}, 0
}

func (rt *Runtime) internalSend(from, to address.Address, method abi.MethodNum, value types.BigInt, params []byte) ([]byte, aerrors.ActorError) {
	ctx, span := trace.StartSpan(rt.ctx, "vmc.Send")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("to", to.String()),
			trace.Int64Attribute("method", int64(method)),
			trace.StringAttribute("value", value.String()),
		)
	}

	msg := &types.Message{
		From:     from,
		To:       to,
		Method:   method,
		Value:    value,
		Params:   params,
		GasLimit: rt.gasAvailable,
	}

	st := rt.state
	if err := st.Snapshot(ctx); err != nil {
		return nil, aerrors.Fatalf("snapshot failed: %s", err)
	}
	defer st.ClearSnapshot()

	ret, errSend, subrt := rt.vm.send(ctx, msg, rt, 0)
	if errSend != nil {
		if errRevert := st.Revert(); errRevert != nil {
			return nil, aerrors.Escalate(errRevert, "failed to revert state tree after failed subcall")
		}
	}

	mr := types.MessageReceipt{
		ExitCode: exitcode.ExitCode(aerrors.RetCode(errSend)),
		Return:   ret,
		GasUsed:  0,
	}

	er := ExecutionResult{
		Msg:    msg,
		MsgRct: &mr,
	}

	if errSend != nil {
		er.Error = errSend.Error()
	}

	if subrt != nil {
		er.Subcalls = subrt.internalExecutions
		rt.numActorsCreated = subrt.numActorsCreated
	}
	rt.internalExecutions = append(rt.internalExecutions, &er)
	return ret, errSend
}

func (rs *Runtime) State() vmr.StateHandle {
	return &shimStateHandle{rs: rs}
}

type shimStateHandle struct {
	rs *Runtime
}

func (ssh *shimStateHandle) Create(obj vmr.CBORMarshaler) {
	c := ssh.rs.Put(obj)
	ssh.rs.stateCommit(EmptyObjectCid, c)
}

func (ssh *shimStateHandle) Readonly(obj vmr.CBORUnmarshaler) {
	act, err := ssh.rs.state.GetActor(ssh.rs.Message().Receiver())
	if err != nil {
		ssh.rs.Abortf(exitcode.SysErrInternal, "failed to get actor for Readonly state: %s", err)
	}
	ssh.rs.Get(act.Head, obj)
}

func (ssh *shimStateHandle) Transaction(obj vmr.CBORer, f func() interface{}) interface{} {
	act, err := ssh.rs.state.GetActor(ssh.rs.Message().Receiver())
	if err != nil {
		ssh.rs.Abortf(exitcode.SysErrInternal, "failed to get actor for Readonly state: %s", err)
	}
	baseState := act.Head
	ssh.rs.Get(baseState, obj)

	out := f()

	c := ssh.rs.Put(obj)

	ssh.rs.stateCommit(baseState, c)

	return out
}

func (rt *Runtime) GetBalance(a address.Address) (types.BigInt, aerrors.ActorError) {
	act, err := rt.state.GetActor(a)
	switch err {
	default:
		return types.EmptyInt, aerrors.Escalate(err, "failed to look up actor balance")
	case hamt.ErrNotFound:
		return types.NewInt(0), nil
	case nil:
		return act.Balance, nil
	}
}

func (rt *Runtime) stateCommit(oldh, newh cid.Cid) aerrors.ActorError {
	// TODO: we can make this more efficient in the future...
	act, err := rt.state.GetActor(rt.Message().Receiver())
	if err != nil {
		rt.Abortf(exitcode.SysErrInternal, "failed to get actor to commit state: %s", err)
	}

	if act.Head != oldh {
		rt.Abortf(exitcode.ErrIllegalState, "failed to update, inconsistent base reference")
	}

	act.Head = newh

	if err := rt.state.SetActor(rt.Message().Receiver(), act); err != nil {
		rt.Abortf(exitcode.SysErrInternal, "failed to set actor in commit state: %s", err)
	}

	return nil
}

func (rt *Runtime) ChargeGas(toUse int64) {
	err := rt.chargeGasSafe(toUse)
	if err != nil {
		panic(err)
	}
}

func (rt *Runtime) chargeGasSafe(toUse int64) aerrors.ActorError {
	rt.gasUsed += toUse
	if rt.gasUsed > rt.gasAvailable {
		return aerrors.Newf(uint8(exitcode.SysErrOutOfGas), "not enough gas: used=%d, available=%d", rt.gasUsed, rt.gasAvailable)
	}
	return nil
}

func (rt *Runtime) Pricelist() Pricelist {
	return rt.pricelist
}

func (rt *Runtime) incrementNumActorsCreated() {
	rt.numActorsCreated++
}
