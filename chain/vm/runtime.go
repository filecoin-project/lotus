package vm

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	samarket "github.com/filecoin-project/specs-actors/actors/builtin/market"
	gruntime "runtime"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	sainit "github.com/filecoin-project/specs-actors/actors/builtin/init"
	sapower "github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
)

type Runtime struct {
	ctx context.Context

	vm        *VM
	state     *state.StateTree
	msg       *types.Message
	vmsg      vmr.Message
	height    abi.ChainEpoch
	cst       cbor.IpldStore
	pricelist Pricelist

	gasAvailable int64
	gasUsed      int64

	sys runtime.Syscalls

	// address that started invoke chain
	origin      address.Address
	originNonce uint64

	executionTrace    types.ExecutionTrace
	numActorsCreated  uint64
	allowInternal     bool
	callerValidated   bool
	lastGasChargeTime time.Time
	lastGasCharge     *types.GasTrace
}

func (rt *Runtime) TotalFilCircSupply() abi.TokenAmount {

	filVested, err := rt.vm.GetVestedFunds(rt.ctx)
	if err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to get vested funds for computing total supply: %s", err)
	}

	rew, err := rt.state.GetActor(builtin.RewardActorAddr)
	if err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to get reward actor for computing total supply: %s", err)
	}

	filMined := types.BigSub(types.FromFil(build.TotalFilecoin), rew.Balance)

	burnt, err := rt.state.GetActor(builtin.BurntFundsActorAddr)
	if err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to get reward actor for computing total supply: %s", err)
	}

	filBurned := burnt.Balance

	market, err := rt.state.GetActor(builtin.StorageMarketActorAddr)
	if err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to get reward actor for computing total supply: %s", err)
	}

	var mst samarket.State
	if err := rt.cst.Get(rt.ctx, market.Head, &mst); err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to get market state: %s", err)
	}

	filMarketLocked := types.BigAdd(mst.TotalClientLockedCollateral, mst.TotalProviderLockedCollateral)
	filMarketLocked = types.BigAdd(filMarketLocked, mst.TotalClientStorageFee)

	power, err := rt.state.GetActor(builtin.StoragePowerActorAddr)
	if err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to get reward actor for computing total supply: %s", err)
	}

	var pst sapower.State
	if err := rt.cst.Get(rt.ctx, power.Head, &pst); err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to get storage power state: %s", err)
	}

	filLocked := types.BigAdd(filMarketLocked, pst.TotalPledgeCollateral)

	ret := types.BigAdd(filVested, filMined)
	ret = types.BigSub(ret, filBurned)
	ret = types.BigSub(ret, filLocked)
	return ret
}

func (rt *Runtime) ResolveAddress(addr address.Address) (ret address.Address, ok bool) {
	r, err := rt.state.LookupID(addr)
	if err != nil {
		if xerrors.Is(err, sainit.ErrAddressNotFound) {
			return address.Undef, false
		}
		panic(aerrors.Fatalf("failed to resolve address %s: %s", addr, err))
	}
	return r, true
}

type notFoundErr interface {
	IsNotFound() bool
}

func (rt *Runtime) Get(c cid.Cid, o vmr.CBORUnmarshaler) bool {
	if err := rt.cst.Get(context.TODO(), c, o); err != nil {
		var nfe notFoundErr
		if xerrors.As(err, &nfe) && nfe.IsNotFound() {
			if xerrors.As(err, new(cbor.SerializationError)) {
				panic(aerrors.Newf(exitcode.ErrSerialization, "failed to unmarshal cbor object %s", err))
			}
			return false
		}

		panic(aerrors.Fatalf("failed to get cbor object %s: %s", c, err))
	}
	return true
}

func (rt *Runtime) Put(x vmr.CBORMarshaler) cid.Cid {
	c, err := rt.cst.Put(context.TODO(), x)
	if err != nil {
		if xerrors.As(err, new(cbor.SerializationError)) {
			panic(aerrors.Newf(exitcode.ErrSerialization, "failed to marshal cbor object %s", err))
		}
		panic(aerrors.Fatalf("failed to put cbor object: %s", err))
	}
	return c
}

var _ vmr.Runtime = (*Runtime)(nil)

func (rt *Runtime) shimCall(f func() interface{}) (rval []byte, aerr aerrors.ActorError) {
	defer func() {
		if r := recover(); r != nil {
			if ar, ok := r.(aerrors.ActorError); ok {
				log.Warnf("VM.Call failure: %+v", ar)
				aerr = ar
				return
			}
			log.Errorf("spec actors failure: %s", r)
			aerr = aerrors.Newf(1, "spec actors failure: %s", r)
		}
	}()

	ret := f()

	if !rt.callerValidated {
		rt.Abortf(exitcode.SysErrorIllegalActor, "Caller MUST be validated during method execution")
	}

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

func (rt *Runtime) Message() vmr.Message {
	return rt.vmsg
}

func (rt *Runtime) ValidateImmediateCallerAcceptAny() {
	rt.abortIfAlreadyValidated()
	return
}

func (rt *Runtime) CurrentBalance() abi.TokenAmount {
	b, err := rt.GetBalance(rt.Message().Receiver())
	if err != nil {
		rt.Abortf(err.RetCode(), "get current balance: %v", err)
	}
	return b
}

func (rt *Runtime) GetActorCodeCID(addr address.Address) (ret cid.Cid, ok bool) {
	act, err := rt.state.GetActor(addr)
	if err != nil {
		if xerrors.Is(err, types.ErrActorNotFound) {
			return cid.Undef, false
		}

		panic(aerrors.Fatalf("failed to get actor: %s", err))
	}

	return act.Code, true
}

func (rt *Runtime) GetRandomness(personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) abi.Randomness {
	res, err := rt.vm.rand.GetRandomness(rt.ctx, personalization, randEpoch, entropy)
	if err != nil {
		panic(aerrors.Fatalf("could not get randomness: %s", err))
	}
	return res
}

func (rt *Runtime) Store() vmr.Store {
	return rt
}

func (rt *Runtime) NewActorAddress() address.Address {
	var b bytes.Buffer
	oa, _ := ResolveToKeyAddr(rt.vm.cstate, rt.vm.cst, rt.origin)
	if err := oa.MarshalCBOR(&b); err != nil { // todo: spec says cbor; why not just bytes?
		panic(aerrors.Fatalf("writing caller address into a buffer: %v", err))
	}

	if err := binary.Write(&b, binary.BigEndian, rt.originNonce); err != nil {
		panic(aerrors.Fatalf("writing nonce address into a buffer: %v", err))
	}
	if err := binary.Write(&b, binary.BigEndian, rt.numActorsCreated); err != nil { // TODO: expose on vm
		panic(aerrors.Fatalf("writing callSeqNum address into a buffer: %v", err))
	}
	addr, err := address.NewActorAddress(b.Bytes())
	if err != nil {
		panic(aerrors.Fatalf("create actor address: %v", err))
	}

	rt.incrementNumActorsCreated()
	return addr
}

func (rt *Runtime) CreateActor(codeID cid.Cid, address address.Address) {
	if !builtin.IsBuiltinActor(codeID) {
		rt.Abortf(exitcode.SysErrorIllegalArgument, "Can only create built-in actors.")
	}

	if builtin.IsSingletonActor(codeID) {
		rt.Abortf(exitcode.SysErrorIllegalArgument, "Can only have one instance of singleton actors.")
	}

	_, err := rt.state.GetActor(address)
	if err == nil {
		rt.Abortf(exitcode.SysErrorIllegalArgument, "Actor address already exists")
	}

	rt.chargeGas(rt.Pricelist().OnCreateActor())

	err = rt.state.SetActor(address, &types.Actor{
		Code:    codeID,
		Head:    EmptyObjectCid,
		Nonce:   0,
		Balance: big.Zero(),
	})
	if err != nil {
		panic(aerrors.Fatalf("creating actor entry: %v", err))
	}
	_ = rt.chargeGasSafe(gasOnActorExec)
}

func (rt *Runtime) DeleteActor(addr address.Address) {
	rt.chargeGas(rt.Pricelist().OnDeleteActor())
	act, err := rt.state.GetActor(rt.Message().Receiver())
	if err != nil {
		if xerrors.Is(err, types.ErrActorNotFound) {
			rt.Abortf(exitcode.SysErrorIllegalActor, "failed to load actor in delete actor: %s", err)
		}
		panic(aerrors.Fatalf("failed to get actor: %s", err))
	}
	if !act.Balance.IsZero() {
		if err := rt.vm.transfer(rt.Message().Receiver(), builtin.BurntFundsActorAddr, act.Balance); err != nil {
			panic(aerrors.Fatalf("failed to transfer balance to burnt funds actor: %s", err))
		}
	}

	if err := rt.state.DeleteActor(rt.Message().Receiver()); err != nil {
		panic(aerrors.Fatalf("failed to delete actor: %s", err))
	}
	_ = rt.chargeGasSafe(gasOnActorExec)
}

func (rt *Runtime) Syscalls() vmr.Syscalls {
	return rt.sys
}

func (rt *Runtime) StartSpan(name string) vmr.TraceSpan {
	panic("implement me")
}

func (rt *Runtime) ValidateImmediateCallerIs(as ...address.Address) {
	rt.abortIfAlreadyValidated()
	imm := rt.Message().Caller()

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

func (rt *Runtime) Abortf(code exitcode.ExitCode, msg string, args ...interface{}) {
	log.Warnf("Abortf: " + fmt.Sprintf(msg, args...))
	panic(aerrors.NewfSkip(2, code, msg, args...))
}

func (rt *Runtime) AbortStateMsg(msg string) {
	panic(aerrors.NewfSkip(3, 101, msg))
}

func (rt *Runtime) ValidateImmediateCallerType(ts ...cid.Cid) {
	rt.abortIfAlreadyValidated()
	callerCid, ok := rt.GetActorCodeCID(rt.Message().Caller())
	if !ok {
		panic(aerrors.Fatalf("failed to lookup code cid for caller"))
	}
	for _, t := range ts {
		if t == callerCid {
			return
		}
	}
	rt.Abortf(exitcode.SysErrForbidden, "caller cid type %q was not one of %v", callerCid, ts)
}

func (rt *Runtime) CurrEpoch() abi.ChainEpoch {
	return rt.height
}

type dumbWrapperType struct {
	val []byte
}

func (dwt *dumbWrapperType) Into(um vmr.CBORUnmarshaler) error {
	return um.UnmarshalCBOR(bytes.NewReader(dwt.val))
}

func (rt *Runtime) Send(to address.Address, method abi.MethodNum, m vmr.CBORMarshaler, value abi.TokenAmount) (vmr.SendReturn, exitcode.ExitCode) {
	if !rt.allowInternal {
		rt.Abortf(exitcode.SysErrorIllegalActor, "runtime.Send() is currently disallowed")
	}
	var params []byte
	if m != nil {
		buf := new(bytes.Buffer)
		if err := m.MarshalCBOR(buf); err != nil {
			rt.Abortf(exitcode.SysErrInvalidParameters, "failed to marshal input parameters: %s", err)
		}
		params = buf.Bytes()
	}

	ret, err := rt.internalSend(rt.Message().Receiver(), to, method, value, params)
	if err != nil {
		if err.IsFatal() {
			panic(err)
		}
		log.Warnf("vmctx send failed: to: %s, method: %d: ret: %d, err: %s", to, method, ret, err)
		return &dumbWrapperType{nil}, err.RetCode()
	}
	_ = rt.chargeGasSafe(gasOnActorExec)
	return &dumbWrapperType{ret}, 0
}

func (rt *Runtime) internalSend(from, to address.Address, method abi.MethodNum, value types.BigInt, params []byte) ([]byte, aerrors.ActorError) {
	start := build.Clock.Now()
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

	ret, errSend, subrt := rt.vm.send(ctx, msg, rt, nil, start)
	if errSend != nil {
		if errRevert := st.Revert(); errRevert != nil {
			return nil, aerrors.Escalate(errRevert, "failed to revert state tree after failed subcall")
		}
	}

	if subrt != nil {
		rt.numActorsCreated = subrt.numActorsCreated
	}
	rt.executionTrace.Subcalls = append(rt.executionTrace.Subcalls, subrt.executionTrace)
	return ret, errSend
}

func (rt *Runtime) State() vmr.StateHandle {
	return &shimStateHandle{rt: rt}
}

type shimStateHandle struct {
	rt *Runtime
}

func (ssh *shimStateHandle) Create(obj vmr.CBORMarshaler) {
	c := ssh.rt.Put(obj)
	// TODO: handle error below
	ssh.rt.stateCommit(EmptyObjectCid, c)
}

func (ssh *shimStateHandle) Readonly(obj vmr.CBORUnmarshaler) {
	act, err := ssh.rt.state.GetActor(ssh.rt.Message().Receiver())
	if err != nil {
		ssh.rt.Abortf(exitcode.SysErrorIllegalArgument, "failed to get actor for Readonly state: %s", err)
	}
	ssh.rt.Get(act.Head, obj)
}

func (ssh *shimStateHandle) Transaction(obj vmr.CBORer, f func() interface{}) interface{} {
	if obj == nil {
		ssh.rt.Abortf(exitcode.SysErrorIllegalActor, "Must not pass nil to Transaction()")
	}

	act, err := ssh.rt.state.GetActor(ssh.rt.Message().Receiver())
	if err != nil {
		ssh.rt.Abortf(exitcode.SysErrorIllegalActor, "failed to get actor for Transaction: %s", err)
	}
	baseState := act.Head
	ssh.rt.Get(baseState, obj)

	ssh.rt.allowInternal = false
	out := f()
	ssh.rt.allowInternal = true

	c := ssh.rt.Put(obj)

	// TODO: handle error below
	ssh.rt.stateCommit(baseState, c)

	return out
}

func (rt *Runtime) GetBalance(a address.Address) (types.BigInt, aerrors.ActorError) {
	act, err := rt.state.GetActor(a)
	switch err {
	default:
		return types.EmptyInt, aerrors.Escalate(err, "failed to look up actor balance")
	case types.ErrActorNotFound:
		return types.NewInt(0), nil
	case nil:
		return act.Balance, nil
	}
}

func (rt *Runtime) stateCommit(oldh, newh cid.Cid) aerrors.ActorError {
	// TODO: we can make this more efficient in the future...
	act, err := rt.state.GetActor(rt.Message().Receiver())
	if err != nil {
		return aerrors.Escalate(err, "failed to get actor to commit state")
	}

	if act.Head != oldh {
		return aerrors.Fatal("failed to update, inconsistent base reference")
	}

	act.Head = newh

	if err := rt.state.SetActor(rt.Message().Receiver(), act); err != nil {
		return aerrors.Fatalf("failed to set actor in commit state: %s", err)
	}

	return nil
}

func (rt *Runtime) finilizeGasTracing() {
	if rt.lastGasCharge != nil {
		rt.lastGasCharge.TimeTaken = time.Since(rt.lastGasChargeTime)
	}
}

// ChargeGas is spec actors function
func (rt *Runtime) ChargeGas(name string, compute int64, virtual int64) {
	err := rt.chargeGasInternal(newGasCharge(name, compute, 0).WithVirtual(virtual, 0), 1)
	if err != nil {
		panic(err)
	}
}

func (rt *Runtime) chargeGas(gas GasCharge) {
	err := rt.chargeGasInternal(gas, 1)
	if err != nil {
		panic(err)
	}
}

func (rt *Runtime) chargeGasFunc(skip int) func(GasCharge) {
	return func(gas GasCharge) {
		err := rt.chargeGasInternal(gas, 1+skip)
		if err != nil {
			panic(err)
		}
	}

}

func (rt *Runtime) chargeGasInternal(gas GasCharge, skip int) aerrors.ActorError {
	toUse := gas.Total()
	var callers [10]uintptr
	cout := gruntime.Callers(2+skip, callers[:])

	now := build.Clock.Now()
	if rt.lastGasCharge != nil {
		rt.lastGasCharge.TimeTaken = now.Sub(rt.lastGasChargeTime)
	}

	gasTrace := types.GasTrace{
		Name:  gas.Name,
		Extra: gas.Extra,

		TotalGas:   toUse,
		ComputeGas: gas.ComputeGas,
		StorageGas: gas.StorageGas,

		TotalVirtualGas:   gas.VirtualCompute*GasComputeMulti + gas.VirtualStorage*GasStorageMulti,
		VirtualComputeGas: gas.VirtualCompute,
		VirtualStorageGas: gas.VirtualStorage,

		Callers: callers[:cout],
	}
	rt.executionTrace.GasCharges = append(rt.executionTrace.GasCharges, &gasTrace)
	rt.lastGasChargeTime = now
	rt.lastGasCharge = &gasTrace

	// overflow safe
	if rt.gasUsed > rt.gasAvailable-toUse {
		rt.gasUsed = rt.gasAvailable
		return aerrors.Newf(exitcode.SysErrOutOfGas, "not enough gas: used=%d, available=%d",
			rt.gasUsed, rt.gasAvailable)
	}
	rt.gasUsed += toUse
	return nil
}

func (rt *Runtime) chargeGasSafe(gas GasCharge) aerrors.ActorError {
	return rt.chargeGasInternal(gas, 1)
}

func (rt *Runtime) Pricelist() Pricelist {
	return rt.pricelist
}

func (rt *Runtime) incrementNumActorsCreated() {
	rt.numActorsCreated++
}

func (rt *Runtime) abortIfAlreadyValidated() {
	if rt.callerValidated {
		rt.Abortf(exitcode.SysErrorIllegalActor, "Method must validate caller identity exactly once")
	}
	rt.callerValidated = true
}

func (rt *Runtime) Log(level vmr.LogLevel, msg string, args ...interface{}) {
	switch level {
	case vmr.DEBUG:
		actorLog.Debugf(msg, args...)
	case vmr.INFO:
		actorLog.Infof(msg, args...)
	case vmr.WARN:
		actorLog.Warnf(msg, args...)
	case vmr.ERROR:
		actorLog.Errorf(msg, args...)
	}
}
