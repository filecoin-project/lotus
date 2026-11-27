package vm

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestBuildApplyRetTrace(t *testing.T) {
	from, err := address.NewIDAddress(100)
	require.NoError(t, err)
	to, err := address.NewIDAddress(101)
	require.NoError(t, err)
	msg := &types.Message{From: from, To: to, Value: big.Zero(), GasFeeCap: big.Zero(), GasPremium: big.Zero()}

	var buf bytes.Buffer
	trace := types.ExecutionTrace{Msg: types.MessageTrace{From: from, To: to, Value: big.Zero(), Method: 2}}
	require.NoError(t, trace.MarshalCBOR(&buf))

	apply := func(t *testing.T, traceBytes []byte) *ApplyRet {
		ret, err := (&FVM{nv: network.Version28}).buildApplyRet(msg, &ffi.ApplyRet{
			Return:         []byte{0x42},
			GasUsed:        1234,
			ExecTraceBytes: traceBytes,
		}, time.Now(), false)
		require.NoError(t, err)
		require.Equal(t, []byte{0x42}, ret.Return)
		require.Equal(t, int64(1234), ret.GasUsed)
		return ret
	}

	t.Run("decodes the trace", func(t *testing.T) {
		ret := apply(t, buf.Bytes())
		require.Equal(t, to, ret.ExecutionTrace.Msg.To)
		require.Equal(t, abi.MethodNum(2), ret.ExecutionTrace.Msg.Method)
	})

	t.Run("drops a trace that fails to decode", func(t *testing.T) {
		ret := apply(t, buf.Bytes()[:buf.Len()-1])
		require.Equal(t, types.ExecutionTrace{}, ret.ExecutionTrace)
	})
}
