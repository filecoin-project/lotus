package eth

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
)

// Note: We intentionally avoid asserting absolute gas constants for 7702.
// Intrinsic overhead values may change; we only test counting/gating behavior elsewhere.

func TestCompute7702IntrinsicOverhead_Monotonic(t *testing.T) {
	// 0 tuples -> 0 overhead
	require.EqualValues(t, 0, compute7702IntrinsicOverhead(0))
	// >0 tuples -> positive overhead and monotonic increase with tuple count
	o1 := compute7702IntrinsicOverhead(1)
	o2 := compute7702IntrinsicOverhead(2)
	require.Greater(t, o1, int64(0))
	require.Greater(t, o2, o1)
}

func TestCountAuthInApplyAndCallParams(t *testing.T) {
	// Build atomic CBOR: [ list-of-tuples, call-tuple ] with list length 3
	var buf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
	for i := 0; i < 3; i++ {
		// write an empty tuple placeholder (array(6) with zeroed children is fine)
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 0))
	}
	// call tuple [to, value, input]
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
	require.NoError(t, cbg.WriteByteArray(&buf, make([]byte, 20)))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{0}))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{}))
	params := buf.Bytes()
	require.Equal(t, 3, countAuthInApplyAndCallParams(params))

	// Non-array should return 0
	buf.Reset()
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 7))
	require.Equal(t, 0, countAuthInApplyAndCallParams(buf.Bytes()))
}

// legacy shape tests removed: we accept atomic/wrapper-only

func TestCountAuthInApplyAndCallParams_EmptyWrapper(t *testing.T) {
	var buf bytes.Buffer
	// atomic [ [], call-tuple ] with list length 0
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 0))
	// call tuple
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
	require.NoError(t, cbg.WriteByteArray(&buf, make([]byte, 20)))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{0}))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{}))
	require.Equal(t, 0, countAuthInApplyAndCallParams(buf.Bytes()))
}
