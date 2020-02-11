package vm

import (
	"bytes"
	"context"
	"fmt"
	"runtime/debug"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type runtimeShim struct {
	vmctx types.VMContext
	vmr.Runtime
}

var _ runtime.Runtime = (*runtimeShim)(nil)

func (rs *runtimeShim) shimCall(f func() interface{}) (rval []byte, aerr aerrors.ActorError) {
	defer func() {
		if r := recover(); r != nil {
			if ar, ok := r.(aerrors.ActorError); ok {
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

func (rs *runtimeShim) Send(to address.Address, method abi.MethodNum, m runtime.CBORMarshaler, value abi.TokenAmount) (vmr.SendReturn, exitcode.ExitCode) {
	buf := new(bytes.Buffer)
	if err := m.MarshalCBOR(buf); err != nil {
		rs.Abort(exitcode.SysErrInvalidParameters, "failed to marshal input parameters: %s", err)
	}

	ret, err := rs.vmctx.Send(to, method, types.BigInt(value), buf.Bytes())
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
	if err := ssh.rs.vmctx.Storage().Commit(actors.EmptyCBOR, c); err != nil {
		panic(err)
	}
}
