package vm

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"runtime/debug"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/ipfs/go-cid"
	"github.com/minio/blake2b-simd"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
)

type runtimeShim struct {
	vmctx types.VMContext
}

func (rs *runtimeShim) Get(c cid.Cid, o vmr.CBORUnmarshaler) bool {
	err := rs.vmctx.Storage().Get(c, o)
	if err != nil { // todo: not found
		rs.Abortf(exitcode.ErrPlaceholder, "storage get: %v", err)
	}
	return true
}

func (rs *runtimeShim) Put(x vmr.CBORMarshaler) cid.Cid {
	c, err := rs.vmctx.Storage().Put(x)
	if err != nil {
		rs.Abortf(exitcode.ErrPlaceholder, "storage put: %v", err) // todo: spec code?
	}
	return c
}

var _ vmr.Runtime = (*runtimeShim)(nil)

func (rs *runtimeShim) shimCall(f func() interface{}) (rval []byte, aerr aerrors.ActorError) {
	defer func() {
		if r := recover(); r != nil {
			if ar, ok := r.(aerrors.ActorError); ok {
				fmt.Println("VM.Call failure: ", ar)
				debug.PrintStack()
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

func (rs *runtimeShim) Message() vmr.Message {
	return rs.vmctx.Message()
}

func (rs *runtimeShim) ValidateImmediateCallerAcceptAny() {
	return
}

func (rs *runtimeShim) CurrentBalance() abi.TokenAmount {
	b, err := rs.vmctx.GetBalance(rs.vmctx.Message().To)
	if err != nil {
		rs.Abortf(exitcode.ExitCode(err.RetCode()), "get current balance: %v", err)
	}
	return b
}

func (rs *runtimeShim) GetActorCodeCID(addr address.Address) (ret cid.Cid, ok bool) {
	ret, err := rs.vmctx.ActorCodeCID(addr)
	if err != nil {
		// todo: notfound
		rs.Abortf(exitcode.ErrPlaceholder, "%v", err)
	}

	return ret, true
}

func (rs *runtimeShim) GetRandomness(personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) abi.Randomness {
	r, err := rs.vmctx.GetRandomness(randEpoch)
	if err != nil {
		rs.Abortf(exitcode.SysErrInternal, "getting randomness: %v", err)
	}

	h := blake2b.New256()
	if err := binary.Write(h, binary.BigEndian, int64(personalization)); err != nil {
		rs.Abortf(exitcode.SysErrInternal, "deriving randomness: %v", err)
	}
	h.Write(r)
	h.Write(entropy)

	return h.Sum(nil)
}

func (rs *runtimeShim) Store() vmr.Store {
	return rs
}

func (rs *runtimeShim) NewActorAddress() address.Address {
	var b bytes.Buffer
	if err := rs.ImmediateCaller().MarshalCBOR(&b); err != nil { // todo: spec says cbor; why not just bytes?
		rs.Abortf(exitcode.ErrSerialization, "writing caller address into a buffer: %v", err)
	}

	var err error
	st, err := rs.vmctx.StateTree()
	if err != nil {
		rs.Abortf(exitcode.SysErrInternal, "getting statetree: %v", err)
	}
	act, err := st.GetActor(rs.vmctx.Origin())
	if err != nil {
		rs.Abortf(exitcode.SysErrInternal, "getting top level actor: %v", err)
	}

	if err := binary.Write(&b, binary.BigEndian, act.Nonce); err != nil {
		rs.Abortf(exitcode.ErrSerialization, "writing nonce address into a buffer: %v", err)
	}
	if err := binary.Write(&b, binary.BigEndian, uint64(0)); err != nil { // TODO: expose on vm
		rs.Abortf(exitcode.ErrSerialization, "writing callSeqNum address into a buffer: %v", err)
	}
	addr, err := address.NewActorAddress(b.Bytes())
	if err != nil {
		rs.Abortf(exitcode.ErrSerialization, "create actor address: %v", err)
	}

	return addr
}

func (rs *runtimeShim) CreateActor(codeId cid.Cid, address address.Address) {
	var err error
	st, err := rs.vmctx.StateTree()
	if err != nil {
		rs.Abortf(exitcode.SysErrInternal, "getting statetree: %v", err)
	}

	err = st.SetActor(address, &types.Actor{
		Code:    codeId,
		Head:    EmptyObjectCid,
		Nonce:   0,
		Balance: big.Zero(),
	})
	if err != nil {
		rs.Abortf(exitcode.SysErrInternal, "creating actor entry: %v", err)
	}

	return
}

func (rs *runtimeShim) DeleteActor() {
	panic("implement me")
}

func (rs *runtimeShim) Syscalls() vmr.Syscalls {
	return rs.vmctx.Sys()
}

func (rs *runtimeShim) StartSpan(name string) vmr.TraceSpan {
	panic("implement me")
}

func (rs *runtimeShim) ValidateImmediateCallerIs(as ...address.Address) {
	for _, a := range as {
		if rs.vmctx.Message().From == a {
			return
		}
	}
	rs.Abortf(exitcode.ErrForbidden, "caller %s is not one of %s", rs.vmctx.Message().From, as)
}

func (rs *runtimeShim) ImmediateCaller() address.Address {
	return rs.vmctx.Message().From
}

func (rs *runtimeShim) Context() context.Context {
	return rs.vmctx.Context()
}

func (rs *runtimeShim) Abortf(code exitcode.ExitCode, msg string, args ...interface{}) {
	panic(aerrors.NewfSkip(2, uint8(code), msg, args...))
}

func (rs *runtimeShim) AbortStateMsg(msg string) {
	panic(aerrors.NewfSkip(3, 101, msg))
}

func (rs *runtimeShim) ValidateImmediateCallerType(...cid.Cid) {
	log.Info("validate caller type is dumb")
}

func (rs *runtimeShim) CurrEpoch() abi.ChainEpoch {
	return rs.vmctx.BlockHeight()
}

type dumbWrapperType struct {
	val []byte
}

func (dwt *dumbWrapperType) Into(um vmr.CBORUnmarshaler) error {
	return um.UnmarshalCBOR(bytes.NewReader(dwt.val))
}

func (rs *runtimeShim) Send(to address.Address, method abi.MethodNum, m vmr.CBORMarshaler, value abi.TokenAmount) (vmr.SendReturn, exitcode.ExitCode) {
	var params []byte
	if m != nil {
		buf := new(bytes.Buffer)
		if err := m.MarshalCBOR(buf); err != nil {
			rs.Abortf(exitcode.SysErrInvalidParameters, "failed to marshal input parameters: %s", err)
		}
		params = buf.Bytes()
	}

	ret, err := rs.vmctx.Send(to, method, types.BigInt(value), params)
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

func (ssh *shimStateHandle) Create(obj vmr.CBORMarshaler) {
	c, err := ssh.rs.vmctx.Storage().Put(obj)
	if err != nil {
		panic(err)
	}
	if err := ssh.rs.vmctx.Storage().Commit(EmptyObjectCid, c); err != nil {
		panic(err)
	}
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
