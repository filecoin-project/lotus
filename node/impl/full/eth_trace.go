package full

import (
	"bytes"

	"github.com/multiformats/go-multicodec"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v12/eam"
	"github.com/filecoin-project/go-state-types/builtin/v12/evm"
	"github.com/filecoin-project/go-state-types/exitcode"

	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
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
	if act.State.Address != nil {
		if addr, err := ethtypes.EthAddressFromFilecoinAddress(*act.State.Address); err == nil {
			return addr
		}
	}
	return ethtypes.EthAddressFromActorID(act.Id)
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

func decodeCreate(msg *types.MessageTrace) (callType string, initcode []byte, err error) {
	switch msg.Method {
	case builtin.MethodsEAM.Create:
		params, err := decodeParams[eam.CreateParams](msg)
		if err != nil {
			return "", nil, err
		}
		return "create", params.Initcode, nil
	case builtin.MethodsEAM.Create2:
		params, err := decodeParams[eam.Create2Params](msg)
		if err != nil {
			return "", nil, err
		}
		return "create2", params.Initcode, nil
	case builtin.MethodsEAM.CreateExternal:
		input, err := decodePayload(msg.Params, msg.ParamsCodec)
		if err != nil {
			return "", nil, err
		}
		return "create", input, nil
	default:
		return "", nil, xerrors.Errorf("unexpected CREATE method %d", msg.Method)
	}
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

	/////////////////////////////////////
	// Step 1: Decode as a native call //
	/////////////////////////////////////

	to := traceToAddress(et.InvokedActor)
	var errMsg string
	if et.MsgRct.ExitCode.IsError() {
		errMsg = et.MsgRct.ExitCode.Error()
	}
	trace := &ethtypes.EthTrace{
		Action: ethtypes.EthTraceAction{
			From:  env.caller,
			To:    &to,
			Gas:   ethtypes.EthUint64(et.Msg.GasLimit),
			Value: ethtypes.EthBigInt(et.Msg.Value),
			Input: encodeFilecoinParamsAsABI(et.Msg.Method, et.Msg.ParamsCodec, et.Msg.Params),
		},
		Result: ethtypes.EthTraceResult{
			GasUsed: ethtypes.EthUint64(et.SumGas().TotalGas),
			Output:  encodeFilecoinReturnAsABI(et.MsgRct.ExitCode, et.MsgRct.ReturnCodec, et.MsgRct.Return),
		},
		TraceAddress: addr,
		Error:        errMsg,
	}

	// Set the assumed call mode. We'll override this if the call ends up looking like a create,
	// delegatecall, etc.
	if et.Msg.ReadOnly {
		trace.SetCallType("staticcall")
	} else {
		trace.SetCallType("call")
	}

	/////////////////////////////////////////////
	// Step 2: Decode as a contract invocation //
	/////////////////////////////////////////////

	// Normal EVM calls. We don't care if the caller/receiver are actually EVM actors, we only
	// care if the call _looks_ like an EVM call. If we fail to decode it as an EVM call, we
	// fallback on interpreting it as a native call.
	if et.Msg.Method == builtin.MethodsEVM.InvokeContract {
		input, err := decodePayload(et.Msg.Params, et.Msg.ParamsCodec)
		if err != nil {
			log.Debugf("failed to decode contract invocation payload: %w", err)
			return trace, et, nil
		}
		output, err := decodePayload(et.MsgRct.Return, et.MsgRct.ReturnCodec)
		if err != nil {
			log.Debugf("failed to decode contract invocation return: %w", err)
			return trace, et, nil
		}
		trace.Action.Input = input
		trace.Result.Output = output
		return trace, et, nil
	}

	/////////////////////////////////////////////
	// Step 3: Decode as a contract deployment //
	/////////////////////////////////////////////

	switch et.Msg.To {
	// NOTE: this will only catch _direct_ calls to the init actor. Calls through the EAM will
	// be caught and _skipped_ below in the next case.
	case builtin.InitActorAddr:
		switch et.Msg.Method {
		case builtin.MethodsInit.Exec, builtin.MethodsInit.Exec4:
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

			// Contract creation has no "to".
			trace.Action.To = nil
			// If we get here, this isn't a native EVM create. Those always go through
			// the EAM. So we have no "real" initcode and must use the sentinel value
			// for "invalid" initcode.
			trace.Action.Input = []byte{0xFE}
			trace.SetCallType("create")
			// Handle the output.
			if et.MsgRct.ExitCode.IsError() {
				// If the sub-call fails, record the reason. It's possible for the
				// call to the init actor to fail, but for the construction to
				// succeed, so we need to check the exit code here.
				if subTrace.MsgRct.ExitCode.IsError() {
					trace.Result.Output = encodeFilecoinReturnAsABI(
						subTrace.MsgRct.ExitCode,
						subTrace.MsgRct.ReturnCodec,
						subTrace.MsgRct.Return,
					)
				} else {
					// Otherwise, output nothing.
					trace.Result.Output = nil
				}
			} else {
				// We're supposed to put the "installed bytecode" here. But this
				// isn't an EVM actor, so we just put some invalid bytecode (this is
				// the answer you'd get if you called EXTCODECOPY on a native
				// non-account actor, anyways).
				trace.Result.Output = []byte{0xFE}
			}
			return trace, subTrace, nil
		}
	case builtin.EthereumAddressManagerActorAddr:
		switch et.Msg.Method {
		case builtin.MethodsEAM.Create, builtin.MethodsEAM.Create2, builtin.MethodsEAM.CreateExternal:
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

			// Contract creation has no "to".
			trace.Action.To = nil

			// Decode inputs & determine create type.
			method, initcode, err := decodeCreate(&et.Msg)
			if err != nil {
				return nil, nil, xerrors.Errorf("EAM called with invalid params, but it still tried to construct the contract: %w", err)
			}
			trace.Action.Input = initcode
			trace.SetCallType(method)

			// Handle the output.
			if et.MsgRct.ExitCode.IsError() {
				if subTrace.MsgRct.ExitCode.IsError() {
					// If we managed to call the constructor, parse/return its
					// revert message.
					output, err := decodePayload(subTrace.MsgRct.Return, subTrace.MsgRct.ReturnCodec)
					if err != nil {
						log.Debugf("EVM actor returned indecipherable create error: %w", err)
						output = encodeFilecoinReturnAsABI(
							subTrace.MsgRct.ExitCode,
							subTrace.MsgRct.ReturnCodec,
							subTrace.MsgRct.Return,
						)
					}
					trace.Result.Output = output
				} else {
					// Otherwise, if we failed before that, we have no revert
					// message.
					trace.Result.Output = nil
				}
			} else {
				// We're _supposed_ to include the contracts bytecode here, but we
				// can't do that reliably (e.g., if some part of the trace reverts).
				// So we don't try and include a sentinel "impossible bytecode"
				// value (the value specified by EIP-3541).
				trace.Result.Output = []byte{0xFE}
			}
			return trace, subTrace, nil
		}
	}

	/////////////////////////////////
	// Step 4: Handle DELEGATECALL //
	/////////////////////////////////

	// EVM contracts cannot call methods in the range 1-1023, only the EVM itself can. So, if we
	// see a call in this range, we know it's an implementation detail of the EVM and not an
	// explicit call by the user.
	//
	// While the EVM calls several methods in this range (some we've already handled above with
	// respect to the EAM), we only care about the ones relevant DELEGATECALL and can _ignore_
	// all the others.
	if env.isEVM && et.Msg.Method > 0 && et.Msg.Method < 1024 {
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
			if env.caller != to {
				return nil, nil, xerrors.Errorf("delegate-call not from & to self: %s != %s", env.caller, to)
			}

			trace.SetCallType("delegatecall")
			trace.Action.To = env.lastByteCode

			dp, err := decodeParams[evm.DelegateCallParams](&et.Msg)
			if err != nil {
				return nil, nil, xerrors.Errorf("failed to decode delegate-call params: %w", err)
			}
			trace.Action.Input = dp.Input

			trace.Result.Output, err = decodePayload(et.MsgRct.Return, et.MsgRct.ReturnCodec)
			if err != nil {
				return nil, nil, xerrors.Errorf("failed to decode delegate-call return: %w", err)
			}

			return trace, et, nil
		}
		// We drop all other "private" calls from FEVM. We _forbid_ explicit calls between 0 and 1024 (exclusive), so any calls in this range must be implementation details.
		return nil, nil, nil
	}

	return trace, et, nil
}
