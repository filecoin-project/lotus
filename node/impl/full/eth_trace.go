package full

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/exitcode"
	"golang.org/x/xerrors"

	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// buildTraces recursively builds the traces for a given ExecutionTrace by walking the subcalls
func buildTraces(ctx context.Context, traces *[]*ethtypes.EthTrace, parent *ethtypes.EthTrace, addr []int, et types.ExecutionTrace, height int64, sa StateAPI) error {
	from, err := lookupEthAddress(ctx, et.Msg.From, sa)
	if err != nil {
		return xerrors.Errorf("buildTraces: failed to lookup from address %s: %w", et.Msg.From, err)
	}
	to, err := lookupEthAddress(ctx, et.Msg.To, sa)
	if err != nil {
		return xerrors.Errorf("buildTraces: failed to lookup to address %s: %w", et.Msg.To, err)
	}

	trace := &ethtypes.EthTrace{
		Action: ethtypes.EthTraceAction{
			From:    from.String(),
			To:      to.String(),
			Gas:     ethtypes.EthUint64(et.Msg.GasLimit),
			Input:   hex.EncodeToString(et.Msg.Params),
			Value:   ethtypes.EthBigInt(et.Msg.Value),
			Method:  et.Msg.Method,
			CodeCid: et.Msg.CodeCid,
		},
		Result: ethtypes.EthTraceResult{
			GasUsed: ethtypes.EthUint64(et.SumGas().TotalGas),
			Output:  hex.EncodeToString(et.MsgRct.Return),
		},
		Subtraces:    len(et.Subcalls),
		TraceAddress: addr,

		Parent: parent,
	}

	trace.SetCallType("call")

	// TODO: is it OK to check this here or is this only specific to certain edge case (evm to evm)?
	if et.Msg.ReadOnly {
		trace.SetCallType("staticcall")
	}

	// there are several edge cases thar require special handling when displaying the traces
	if parent != nil {
		// Handle Native calls
		//
		// When an EVM actor is invoked with a method number above 1023 that's not frc42(InvokeEVM)
		// then we need to format native calls in a way that makes sense to Ethereum tooling (convert
		// the input & output to solidity ABI format).
		if builtinactors.IsEvmActor(parent.Action.CodeCid) &&
			et.Msg.Method > 1023 &&
			et.Msg.Method != builtin.MethodsEVM.InvokeContract {
			log.Debugf("found Native call! method:%d, code:%s, height:%d", et.Msg.Method, et.Msg.CodeCid.String(), height)
			input, err := handleFilecoinMethodInput(et.Msg.Method, et.Msg.ParamsCodec, et.Msg.Params)
			if err != nil {
				return err
			}
			trace.Action.Input = hex.EncodeToString(input)
			output, err := handleFilecoinMethodOutput(et.MsgRct.ExitCode, et.MsgRct.ReturnCodec, et.MsgRct.Return)
			if err != nil {
				return err
			}
			trace.Result.Output = hex.EncodeToString(output)
		}

		// Handle Native actor creation
		//
		// Actor A calls to the init actor on method 2 and The init actor creates the target actor B then calls it on method 1
		if parent.Action.To == builtin.InitActorAddr.String() &&
			parent.Action.Method == builtin.MethodsInit.Exec &&
			et.Msg.Method == builtin.MethodConstructor {
			log.Debugf("Native actor creation! method:%d, code:%s, height:%d", et.Msg.Method, et.Msg.CodeCid.String(), height)
			parent.SetCallType("create")
			parent.Action.To = et.Msg.To.String()
			parent.Action.Input = hex.EncodeToString([]byte{0x0, 0x0, 0x0, 0xFE})
			parent.Result.Output = ""

			// there should never be any subcalls when creating a native actor
			return nil
		}

		// Handle EVM contract creation
		//
		// To detect EVM contract creation we need to check for the following sequence of events:
		//
		// 1) EVM contract A calls the EAM (Ethereum Address Manager) on method 2 (create) or 3 (create2).
		// 2) The EAM calls the init actor on method 3 (Exec4).
		// 3) The init actor creates the target actor B then calls it on method 1.
		if parent.Parent != nil {
			calledCreateOnEAM := parent.Parent.Action.To == builtin.EthereumAddressManagerActorAddr.String() &&
				(parent.Parent.Action.Method == builtin.MethodsEAM.Create || parent.Parent.Action.Method == builtin.MethodsEAM.Create2)
			eamCalledInitOnExec4 := parent.Action.To == builtin.InitActorAddr.String() &&
				parent.Action.Method == builtin.MethodsInit.Exec4
			initCreatedActor := trace.Action.Method == builtin.MethodConstructor

			if calledCreateOnEAM && eamCalledInitOnExec4 && initCreatedActor {
				log.Debugf("EVM contract creation method:%d, code:%s, height:%d", et.Msg.Method, et.Msg.CodeCid.String(), height)

				if parent.Parent.Action.Method == builtin.MethodsEAM.Create {
					parent.Parent.SetCallType("create")
				} else {
					parent.Parent.SetCallType("create2")
				}

				// update the parent.parent to make this
				parent.Parent.Action.To = trace.Action.To
				parent.Parent.Subtraces = 0

				// delete the parent (the EAM) and skip the current trace (init)
				*traces = (*traces)[:len(*traces)-1]

				return nil
			}
		}

		// Handle EVM call special casing
		//
		// Any outbound call from an EVM actor on methods 1-1023 are side-effects from EVM instructions
		// and should be dropped from the trace.
		if builtinactors.IsEvmActor(parent.Action.CodeCid) &&
			et.Msg.Method > 0 &&
			et.Msg.Method <= 1023 {
			log.Debugf("found outbound call from an EVM actor on method 1-1023 method:%d, code:%s, height:%d", et.Msg.Method, parent.Action.CodeCid.String(), height)
			// TODO: if I handle this case and drop this call from the trace then I am not able to detect delegate calls
		}

		// EVM -> EVM calls
		//
		// Check for normal EVM to EVM calls and decode the params and return values
		if builtinactors.IsEvmActor(parent.Action.CodeCid) &&
			builtinactors.IsEthAccountActor(et.Msg.CodeCid) &&
			et.Msg.Method == builtin.MethodsEVM.InvokeContract {
			log.Debugf("found evm to evm call, code:%s, height: %d", et.Msg.CodeCid.String(), height)
			input, err := handleFilecoinMethodInput(et.Msg.Method, et.Msg.ParamsCodec, et.Msg.Params)
			if err != nil {
				return err
			}
			trace.Action.Input = hex.EncodeToString(input)
			output, err := handleFilecoinMethodOutput(et.MsgRct.ExitCode, et.MsgRct.ReturnCodec, et.MsgRct.Return)
			if err != nil {
				return err
			}
			trace.Result.Output = hex.EncodeToString(output)
		}

		// Handle delegate calls
		//
		// 1) Look for trace from an EVM actor to itself on InvokeContractDelegate, method 6.
		// 2) Check that the previous trace calls another actor on method 3 (GetByteCode) and they are at the same level (same parent)
		// 3) Treat this as a delegate call to actor A.
		if trace.Action.From == trace.Action.To &&
			trace.Action.Method == builtin.MethodsEVM.InvokeContractDelegate &&
			len(*traces) > 0 {
			log.Debugf("found delegate call, height: %d", height)
			prev := (*traces)[len(*traces)-1]
			if prev.Action.From == trace.Action.From && prev.Action.Method == builtin.MethodsEVM.GetBytecode && prev.Parent == trace.Parent {
				trace.SetCallType("delegatecall")
				trace.Action.To = prev.Action.To
			}
		}
	}

	*traces = append(*traces, trace)

	for i, call := range et.Subcalls {
		err := buildTraces(ctx, traces, trace, append(addr, i), call, height, sa)
		if err != nil {
			return err
		}
	}

	return nil
}

func writePadded(w io.Writer, data any, size int) error {
	tmp := &bytes.Buffer{}

	// first write data to tmp buffer to get the size
	err := binary.Write(tmp, binary.BigEndian, data)
	if err != nil {
		return fmt.Errorf("writePadded: failed writing tmp data to buffer: %w", err)
	}

	if tmp.Len() > size {
		return fmt.Errorf("writePadded: data is larger than size")
	}

	// write tailing zeros to pad up to size
	cnt := size - tmp.Len()
	for i := 0; i < cnt; i++ {
		err = binary.Write(w, binary.BigEndian, uint8(0))
		if err != nil {
			return fmt.Errorf("writePadded: failed writing tailing zeros to buffer: %w", err)
		}
	}

	// finally write the actual value
	err = binary.Write(w, binary.BigEndian, tmp.Bytes())
	if err != nil {
		return fmt.Errorf("writePadded: failed writing data to buffer: %w", err)
	}

	return nil
}

func handleFilecoinMethodInput(method abi.MethodNum, codec uint64, params []byte) ([]byte, error) {
	NATIVE_METHOD_SELECTOR := []byte{0x86, 0x8e, 0x10, 0xc4}
	EVM_WORD_SIZE := 32

	staticArgs := []uint64{
		uint64(method),
		codec,
		uint64(EVM_WORD_SIZE) * 3,
		uint64(len(params)),
	}
	totalWords := len(staticArgs) + (len(params) / EVM_WORD_SIZE)
	if len(params)%EVM_WORD_SIZE != 0 {
		totalWords++
	}
	len := 4 + totalWords*EVM_WORD_SIZE

	w := &bytes.Buffer{}
	err := binary.Write(w, binary.BigEndian, NATIVE_METHOD_SELECTOR)
	if err != nil {
		return nil, fmt.Errorf("handleFilecoinMethodInput: failed writing method selector: %w", err)
	}

	for _, arg := range staticArgs {
		err := writePadded(w, arg, 32)
		if err != nil {
			return nil, fmt.Errorf("handleFilecoinMethodInput: %w", err)
		}
	}
	err = binary.Write(w, binary.BigEndian, params)
	if err != nil {
		return nil, fmt.Errorf("handleFilecoinMethodInput: failed writing params: %w", err)
	}
	remain := len - w.Len()
	for i := 0; i < remain; i++ {
		err = binary.Write(w, binary.BigEndian, uint8(0))
		if err != nil {
			return nil, fmt.Errorf("handleFilecoinMethodInput: failed writing tailing zeros: %w", err)
		}
	}

	return w.Bytes(), nil
}

func handleFilecoinMethodOutput(exitCode exitcode.ExitCode, codec uint64, data []byte) ([]byte, error) {
	w := &bytes.Buffer{}

	values := []interface{}{uint32(exitCode), codec, uint32(w.Len()), uint32(len(data))}
	for _, v := range values {
		err := writePadded(w, v, 32)
		if err != nil {
			return nil, fmt.Errorf("handleFilecoinMethodOutput: %w", err)
		}
	}

	err := binary.Write(w, binary.BigEndian, data)
	if err != nil {
		return nil, fmt.Errorf("handleFilecoinMethodOutput: failed writing data: %w", err)
	}

	return w.Bytes(), nil
}
