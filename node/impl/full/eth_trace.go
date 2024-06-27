package full

import (
	"bytes"
	"fmt"

	"github.com/multiformats/go-multicodec"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	eam12 "github.com/filecoin-project/go-state-types/builtin/v12/eam"
	evm12 "github.com/filecoin-project/go-state-types/builtin/v12/evm"
	init12 "github.com/filecoin-project/go-state-types/builtin/v12/init"
	"github.com/filecoin-project/go-state-types/exitcode"

	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/evm"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// decodePayload is a utility function which decodes the payload using the given codec
func decodePayload(payload []byte, codec uint64) (ethtypes.EthBytes, error) {
	switch multicodec.Code(codec) {
	case multicodec.Identity:
		return nil, nil
	case multicodec.DagCbor, multicodec.Cbor:
		buf, err := cbg.ReadByteArray(bytes.NewReader(payload), uint64(len(payload)))
		if err != nil {
			return nil, xerrors.Errorf("decodePayload: failed to decode cbor payload: %w", err)
		}
		return buf, nil
	case multicodec.Raw:
		return ethtypes.EthBytes(payload), nil
	}

	return nil, xerrors.Errorf("decodePayload: unsupported codec: %d", codec)
}

func decodeParams[P any, T interface {
	*P
	cbg.CBORUnmarshaler
}](msg *types.MessageTrace) (T, error) {
	var params T = new(P)
	switch msg.ParamsCodec {
	case uint64(multicodec.DagCbor), uint64(multicodec.Cbor):
	default:
		return nil, xerrors.Errorf("Method called with unexpected codec %d", msg.ParamsCodec)
	}

	if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
		return nil, xerrors.Errorf("failed to decode params: %w", err)
	}

	return params, nil
}

func decodeReturn[R any, T interface {
	*R
	cbg.CBORUnmarshaler
}](ret *types.ReturnTrace) (T, error) {
	var retval T = new(R)
	switch ret.ReturnCodec {
	case uint64(multicodec.DagCbor), uint64(multicodec.Cbor):
	default:
		return nil, xerrors.Errorf("Method returned an unexpected codec %d", ret.ReturnCodec)
	}

	if err := retval.UnmarshalCBOR(bytes.NewReader(ret.Return)); err != nil {
		return nil, xerrors.Errorf("failed to decode return value: %w", err)
	}

	return retval, nil
}

func find[T any](values []T, cb func(t *T) *T) *T {
	for i := range values {
		if o := cb(&values[i]); o != nil {
			return o
		}
	}
	return nil
}

type environment struct {
	caller        ethtypes.EthAddress
	isEVM         bool
	subtraceCount int
	traces        []*ethtypes.EthTrace
	lastByteCode  *ethtypes.EthAddress
}

func baseEnvironment(st *state.StateTree, from address.Address) (*environment, error) {
	sender, err := lookupEthAddress(from, st)
	if err != nil {
		return nil, xerrors.Errorf("top-level message sender %s s could not be found: %w", from, err)
	}
	return &environment{caller: sender}, nil
}

func traceToAddress(act *types.ActorTrace) ethtypes.EthAddress {
	if act.State.DelegatedAddress != nil {
		if addr, err := ethtypes.EthAddressFromFilecoinAddress(*act.State.DelegatedAddress); err == nil {
			return addr
		}
	}
	return ethtypes.EthAddressFromActorID(act.Id)
}

// traceIsEVMOrEAM returns true if the trace is a call to an EVM or EAM actor.
func traceIsEVMOrEAM(et *types.ExecutionTrace) bool {
	if et.InvokedActor == nil {
		return false
	}
	return builtinactors.IsEvmActor(et.InvokedActor.State.Code) ||
		et.InvokedActor.Id != abi.ActorID(builtin.EthereumAddressManagerActorID)
}

func traceErrMsg(et *types.ExecutionTrace) string {
	code := et.MsgRct.ExitCode

	if code.IsSuccess() {
		return ""
	}

	// EVM tools often expect this literal string.
	if code == exitcode.SysErrOutOfGas {
		return "out of gas"
	}

	// indicate when we have a "system" error.
	if code < exitcode.FirstActorErrorCode {
		return fmt.Sprintf("vm error: %s", code)
	}

	// handle special exit codes from the EVM/EAM.
	if traceIsEVMOrEAM(et) {
		switch code {
		case evm.ErrReverted:
			return "Reverted" // capitalized for compatibility
		case evm.ErrInvalidInstruction:
			return "invalid instruction"
		case evm.ErrUndefinedInstruction:
			return "undefined instruction"
		case evm.ErrStackUnderflow:
			return "stack underflow"
		case evm.ErrStackOverflow:
			return "stack overflow"
		case evm.ErrIllegalMemoryAccess:
			return "illegal memory access"
		case evm.ErrBadJumpdest:
			return "invalid jump destination"
		case evm.ErrSelfdestructFailed:
			return "self destruct failed"
		}
	}
	// everything else...
	return fmt.Sprintf("actor error: %s", code.Error())
}

// buildTraces recursively builds the traces for a given ExecutionTrace by walking the subcalls
func buildTraces(env *environment, addr []int, et *types.ExecutionTrace) error {
	trace, recurseInto, err := buildTrace(env, addr, et)
	if err != nil {
		return xerrors.Errorf("at trace %v: %w", addr, err)
	}

	if trace != nil {
		env.traces = append(env.traces, trace)
		env.subtraceCount++
	}

	// Skip if there's nothing more to do and/or `buildTrace` told us to skip this one.
	if recurseInto == nil || recurseInto.InvokedActor == nil || len(recurseInto.Subcalls) == 0 {
		return nil
	}

	subEnv := &environment{
		caller: traceToAddress(recurseInto.InvokedActor),
		isEVM:  builtinactors.IsEvmActor(recurseInto.InvokedActor.State.Code),
		traces: env.traces,
	}
	// Set capacity to the length so each `append` below creates a new slice. Otherwise, we'll
	// end up repeatedly mutating previous paths.
	addr = addr[:len(addr):len(addr)]
	for i := range recurseInto.Subcalls {
		err := buildTraces(subEnv, append(addr, subEnv.subtraceCount), &recurseInto.Subcalls[i])
		if err != nil {
			return err
		}
	}
	trace.Subtraces = subEnv.subtraceCount
	env.traces = subEnv.traces

	return nil
}

// buildTrace processes the passed execution trace and updates the environment, if necessary.
//
// On success, it returns a trace to add (or nil to skip) and the trace recurse into (or nil to skip).
func buildTrace(env *environment, addr []int, et *types.ExecutionTrace) (*ethtypes.EthTrace, *types.ExecutionTrace, error) {
	// This function first assumes that the call is a "native" call, then handles all the "not
	// native" cases. If we get any unexpected results in any of these special cases, we just
	// keep the "native" interpretation and move on.
	//
	// 1. If we're invoking a contract (even if the caller is a native account/actor), we
	//    attempt to decode the params/return value as a contract invocation.
	// 2. If we're calling the EAM and/or init actor, we try to treat the call as a CREATE.
	// 3. Finally, if the caller is an EVM smart contract and it's calling a "private" (1-1023)
	//    method, we know something special is going on. We look for calls related to
	//    DELEGATECALL and drop everything else (everything else includes calls triggered by,
	//    e.g., EXTCODEHASH).

	// If we don't have sufficient funds, or we have a fatal error, or we have some
	// other syscall error: skip the entire trace to mimic Ethereum (Ethereum records
	// traces _after_ checking things like this).
	//
	// NOTE: The FFI currently folds all unknown syscall errors into "sys assertion
	// failed" which is turned into SysErrFatal.
	if len(addr) > 0 {
		switch et.MsgRct.ExitCode {
		case exitcode.SysErrInsufficientFunds, exitcode.SysErrFatal:
			return nil, nil, nil
		}
	}

	// We may fail before we can even invoke the actor. In that case, we have no 100% reliable
	// way of getting its address (e.g., due to reverts) so we're just going to drop the entire
	// trace. This is OK (ish) because the call never really "happened".
	if et.InvokedActor == nil {
		return nil, nil, nil
	}

	// Step 2: Decode as a contract invocation
	//
	// Normal EVM calls. We don't care if the caller/receiver are actually EVM actors, we only
	// care if the call _looks_ like an EVM call. If we fail to decode it as an EVM call, we
	// fallback on interpreting it as a native call.
	if et.Msg.Method == builtin.MethodsEVM.InvokeContract {
		return traceEVMCall(env, addr, et)
	}

	// Step 3: Decode as a contract deployment
	switch et.Msg.To {
	// NOTE: this will only catch _direct_ calls to the init actor. Calls through the EAM will
	// be caught and _skipped_ below in the next case.
	case builtin.InitActorAddr:
		switch et.Msg.Method {
		case builtin.MethodsInit.Exec, builtin.MethodsInit.Exec4:
			return traceNativeCreate(env, addr, et)
		}
	case builtin.EthereumAddressManagerActorAddr:
		switch et.Msg.Method {
		case builtin.MethodsEAM.Create, builtin.MethodsEAM.Create2, builtin.MethodsEAM.CreateExternal:
			return traceEthCreate(env, addr, et)
		}
	}

	// Step 4: Handle DELEGATECALL
	//
	// EVM contracts cannot call methods in the range 1-1023, only the EVM itself can. So, if we
	// see a call in this range, we know it's an implementation detail of the EVM and not an
	// explicit call by the user.
	//
	// While the EVM calls several methods in this range (some we've already handled above with
	// respect to the EAM), we only care about the ones relevant DELEGATECALL and can _ignore_
	// all the others.
	if env.isEVM && et.Msg.Method > 0 && et.Msg.Method < 1024 {
		return traceEVMPrivate(env, addr, et)
	}

	return traceNativeCall(env, addr, et), et, nil
}

// Build an EthTrace for a "call" with the given input & output.
func traceCall(env *environment, addr []int, et *types.ExecutionTrace, input, output ethtypes.EthBytes) *ethtypes.EthTrace {
	to := traceToAddress(et.InvokedActor)
	callType := "call"
	if et.Msg.ReadOnly {
		callType = "staticcall"
	}
	return &ethtypes.EthTrace{
		Type: "call",
		Action: &ethtypes.EthCallTraceAction{
			CallType: callType,
			From:     env.caller,
			To:       to,
			Gas:      ethtypes.EthUint64(et.Msg.GasLimit),
			Value:    ethtypes.EthBigInt(et.Msg.Value),
			Input:    input,
		},
		Result: &ethtypes.EthCallTraceResult{
			GasUsed: ethtypes.EthUint64(et.SumGas().TotalGas),
			Output:  output,
		},
		TraceAddress: addr,
		Error:        traceErrMsg(et),
	}
}

// Build an EthTrace for a "call", parsing the inputs & outputs as a "native" FVM call.
func traceNativeCall(env *environment, addr []int, et *types.ExecutionTrace) *ethtypes.EthTrace {
	return traceCall(env, addr, et,
		encodeFilecoinParamsAsABI(et.Msg.Method, et.Msg.ParamsCodec, et.Msg.Params),
		encodeFilecoinReturnAsABI(et.MsgRct.ExitCode, et.MsgRct.ReturnCodec, et.MsgRct.Return),
	)
}

// Build an EthTrace for a "call", parsing the inputs & outputs as an EVM call (falling back on
// treating it as a native call).
func traceEVMCall(env *environment, addr []int, et *types.ExecutionTrace) (*ethtypes.EthTrace, *types.ExecutionTrace, error) {
	input, err := decodePayload(et.Msg.Params, et.Msg.ParamsCodec)
	if err != nil {
		log.Debugf("failed to decode contract invocation payload: %w", err)
		return traceNativeCall(env, addr, et), et, nil
	}
	output, err := decodePayload(et.MsgRct.Return, et.MsgRct.ReturnCodec)
	if err != nil {
		log.Debugf("failed to decode contract invocation return: %w", err)
		return traceNativeCall(env, addr, et), et, nil
	}
	return traceCall(env, addr, et, input, output), et, nil
}

// Build an EthTrace for a native "create" operation. This should only be called with an
// ExecutionTrace is an Exec or Exec4 method invocation on the Init actor.
func traceNativeCreate(env *environment, addr []int, et *types.ExecutionTrace) (*ethtypes.EthTrace, *types.ExecutionTrace, error) {
	if et.Msg.ReadOnly {
		// "create" isn't valid in a staticcall, so we just skip this trace
		// (couldn't have created an actor anyways).
		// This mimic's the EVM: it doesn't trace CREATE calls when in
		// read-only mode.
		return nil, nil, nil
	}

	subTrace := find(et.Subcalls, func(c *types.ExecutionTrace) *types.ExecutionTrace {
		if c.Msg.Method == builtin.MethodConstructor {
			return c
		}
		return nil
	})
	if subTrace == nil {
		// If we succeed in calling Exec/Exec4 but don't even try to construct
		// something, we have a bug in our tracing logic or a mismatch between our
		// tracing logic and the actors.
		if et.MsgRct.ExitCode.IsSuccess() {
			return nil, nil, xerrors.Errorf("successful Exec/Exec4 call failed to call a constructor")
		}
		// Otherwise, this can happen if creation fails early (bad params,
		// out of gas, contract already exists, etc.). The EVM wouldn't
		// trace such cases, so we don't either.
		//
		// NOTE: It's actually impossible to run out of gas before calling
		// initcode in the EVM (without running out of gas in the calling
		// contract), but this is an equivalent edge-case to InvokedActor
		// being nil, so we treat it the same way and skip the entire
		// operation.
		return nil, nil, nil
	}

	// Native actors that aren't the EAM can attempt to call Exec4, but such
	// call should fail immediately without ever attempting to construct an
	// actor. I'm catching this here because it likely means that there's a bug
	// in our trace-conversion logic.
	if et.Msg.Method == builtin.MethodsInit.Exec4 {
		return nil, nil, xerrors.Errorf("direct call to Exec4 successfully called a constructor!")
	}

	var output ethtypes.EthBytes
	var createdAddr *ethtypes.EthAddress
	if et.MsgRct.ExitCode.IsSuccess() {
		// We're supposed to put the "installed bytecode" here. But this
		// isn't an EVM actor, so we just put some invalid bytecode (this is
		// the answer you'd get if you called EXTCODECOPY on a native
		// non-account actor, anyways).
		output = []byte{0xFE}

		// Extract the address of the created actor from the return value.
		initReturn, err := decodeReturn[init12.ExecReturn](&et.MsgRct)
		if err != nil {
			return nil, nil, xerrors.Errorf("failed to decode init params after a successful Init.Exec call: %w", err)
		}
		actorId, err := address.IDFromAddress(initReturn.IDAddress)
		if err != nil {
			return nil, nil, xerrors.Errorf("failed to extract created actor ID from address: %w", err)
		}
		ethAddr := ethtypes.EthAddressFromActorID(abi.ActorID(actorId))
		createdAddr = &ethAddr
	}

	return &ethtypes.EthTrace{
		Type: "create",
		Action: &ethtypes.EthCreateTraceAction{
			From:  env.caller,
			Gas:   ethtypes.EthUint64(et.Msg.GasLimit),
			Value: ethtypes.EthBigInt(et.Msg.Value),
			// If we get here, this isn't a native EVM create. Those always go through
			// the EAM. So we have no "real" initcode and must use the sentinel value
			// for "invalid" initcode.
			Init: []byte{0xFE},
		},
		Result: &ethtypes.EthCreateTraceResult{
			GasUsed: ethtypes.EthUint64(et.SumGas().TotalGas),
			Address: createdAddr,
			Code:    output,
		},
		TraceAddress: addr,
		Error:        traceErrMsg(et),
	}, subTrace, nil
}

// Assert that these are all identical so we can simplify the below code and decode once.
var _ *eam12.Return = (*eam12.Return)((*eam12.CreateReturn)(nil))
var _ *eam12.Return = (*eam12.Return)((*eam12.Create2Return)(nil))
var _ *eam12.Return = (*eam12.Return)((*eam12.CreateExternalReturn)(nil))

// Decode the parameters and return value of an EVM smart contract creation through the EAM. This
// should only be called with an ExecutionTrace for a Create, Create2, or CreateExternal method
// invocation on the EAM.
func decodeCreateViaEAM(et *types.ExecutionTrace) (initcode []byte, addr *ethtypes.EthAddress, err error) {
	switch et.Msg.Method {
	case builtin.MethodsEAM.Create:
		params, err := decodeParams[eam12.CreateParams](&et.Msg)
		if err != nil {
			return nil, nil, err
		}
		initcode = params.Initcode
	case builtin.MethodsEAM.Create2:
		params, err := decodeParams[eam12.Create2Params](&et.Msg)
		if err != nil {
			return nil, nil, err
		}
		initcode = params.Initcode
	case builtin.MethodsEAM.CreateExternal:
		input, err := decodePayload(et.Msg.Params, et.Msg.ParamsCodec)
		if err != nil {
			return nil, nil, err
		}
		initcode = input
	default:
		return nil, nil, xerrors.Errorf("unexpected CREATE method %d", et.Msg.Method)
	}
	ret, err := decodeReturn[eam12.CreateReturn](&et.MsgRct)
	if err != nil {
		return nil, (*ethtypes.EthAddress)(&ret.EthAddress), err
	}
	return initcode, (*ethtypes.EthAddress)(&ret.EthAddress), nil
}

// Build an EthTrace for an EVM "create" operation. This should only be called with an
// ExecutionTrace for a Create, Create2, or CreateExternal method invocation on the EAM.
func traceEthCreate(env *environment, addr []int, et *types.ExecutionTrace) (*ethtypes.EthTrace, *types.ExecutionTrace, error) {
	// Same as the Init actor case above, see the comment there.
	if et.Msg.ReadOnly {
		return nil, nil, nil
	}

	// Look for a call to either a constructor or the EVM's resurrect method.
	subTrace := find(et.Subcalls, func(et *types.ExecutionTrace) *types.ExecutionTrace {
		if et.Msg.To == builtinactors.InitActorAddr {
			return find(et.Subcalls, func(et *types.ExecutionTrace) *types.ExecutionTrace {
				if et.Msg.Method == builtinactors.MethodConstructor {
					return et
				}
				return nil
			})
		}
		if et.Msg.Method == builtin.MethodsEVM.Resurrect {
			return et
		}
		return nil
	})

	// Same as the Init actor case above, see the comment there.
	if subTrace == nil {
		if et.MsgRct.ExitCode.IsSuccess() {
			return nil, nil, xerrors.Errorf("successful Create/Create2 call failed to call a constructor")
		}
		return nil, nil, nil
	}

	// Decode inputs & determine create type.
	initcode, createdAddr, err := decodeCreateViaEAM(et)
	if err != nil {
		return nil, nil, xerrors.Errorf("EAM called with invalid params or returned an invalid result, but it still tried to construct the contract: %w", err)
	}

	var output ethtypes.EthBytes
	// Handle the output.
	switch et.MsgRct.ExitCode {
	case 0: // success
		// We're _supposed_ to include the contracts bytecode here, but we
		// can't do that reliably (e.g., if some part of the trace reverts).
		// So we don't try and include a sentinel "impossible bytecode"
		// value (the value specified by EIP-3541).
		output = []byte{0xFE}
	case 33: // Reverted, parse the revert message.
		// If we managed to call the constructor, parse/return its revert message. If we
		// fail, we just return no output.
		output, _ = decodePayload(subTrace.MsgRct.Return, subTrace.MsgRct.ReturnCodec)
	}

	return &ethtypes.EthTrace{
		Type: "create",
		Action: &ethtypes.EthCreateTraceAction{
			From:  env.caller,
			Gas:   ethtypes.EthUint64(et.Msg.GasLimit),
			Value: ethtypes.EthBigInt(et.Msg.Value),
			Init:  initcode,
		},
		Result: &ethtypes.EthCreateTraceResult{
			GasUsed: ethtypes.EthUint64(et.SumGas().TotalGas),
			Address: createdAddr,
			Code:    output,
		},
		TraceAddress: addr,
		Error:        traceErrMsg(et),
	}, subTrace, nil
}

// Build an EthTrace for a "private" method invocation from the EVM. This should only be called with
// an ExecutionTrace from an EVM instance and on a method between 1 and 1023 inclusive.
func traceEVMPrivate(env *environment, addr []int, et *types.ExecutionTrace) (*ethtypes.EthTrace, *types.ExecutionTrace, error) {
	// The EVM actor implements DELEGATECALL by:
	//
	// 1. Asking the callee for its bytecode by calling it on the GetBytecode method.
	// 2. Recursively invoking the currently executing contract on the
	//    InvokeContractDelegate method.
	//
	// The code below "reconstructs" that delegate call by:
	//
	// 1. Remembering the last contract on which we called GetBytecode.
	// 2. Treating the contract invoked in step 1 as the DELEGATECALL receiver.
	//
	// Note, however: GetBytecode will be called, e.g., if the user invokes the
	// EXTCODECOPY instruction. It's not an error to see multiple GetBytecode calls
	// before we see an InvokeContractDelegate.
	switch et.Msg.Method {
	case builtin.MethodsEVM.GetBytecode:
		// NOTE: I'm not checking anything about the receiver here. The EVM won't
		// DELEGATECALL any non-EVM actor, but there's no need to encode that fact
		// here in case we decide to loosen this up in the future.
		if et.MsgRct.ExitCode.IsSuccess() {
			to := traceToAddress(et.InvokedActor)
			env.lastByteCode = &to
		} else {
			env.lastByteCode = nil
		}
		return nil, nil, nil
	case builtin.MethodsEVM.InvokeContractDelegate:
		// NOTE: We return errors in all the failure cases below instead of trying
		// to continue because the caller is an EVM actor. If something goes wrong
		// here, there's a bug in our EVM implementation.

		// Handle delegate calls
		//
		// 1) Look for trace from an EVM actor to itself on InvokeContractDelegate,
		//    method 6.
		// 2) Check that the previous trace calls another actor on method 3
		//    (GetByteCode) and they are at the same level (same parent)
		// 3) Treat this as a delegate call to actor A.
		if env.lastByteCode == nil {
			return nil, nil, xerrors.Errorf("unknown bytecode for delegate call")
		}

		if to := traceToAddress(et.InvokedActor); env.caller != to {
			return nil, nil, xerrors.Errorf("delegate-call not from & to self: %s != %s", env.caller, to)
		}

		dp, err := decodeParams[evm12.DelegateCallParams](&et.Msg)
		if err != nil {
			return nil, nil, xerrors.Errorf("failed to decode delegate-call params: %w", err)
		}

		output, err := decodePayload(et.MsgRct.Return, et.MsgRct.ReturnCodec)
		if err != nil {
			return nil, nil, xerrors.Errorf("failed to decode delegate-call return: %w", err)
		}

		return &ethtypes.EthTrace{
			Type: "call",
			Action: &ethtypes.EthCallTraceAction{
				CallType: "delegatecall",
				From:     env.caller,
				To:       *env.lastByteCode,
				Gas:      ethtypes.EthUint64(et.Msg.GasLimit),
				Value:    ethtypes.EthBigInt(et.Msg.Value),
				Input:    dp.Input,
			},
			Result: &ethtypes.EthCallTraceResult{
				GasUsed: ethtypes.EthUint64(et.SumGas().TotalGas),
				Output:  output,
			},
			TraceAddress: addr,
			Error:        traceErrMsg(et),
		}, et, nil
	}
	// We drop all other "private" calls from FEVM. We _forbid_ explicit calls between 0 and
	// 1024 (exclusive), so any calls in this range must be implementation details.
	return nil, nil, nil
}
