package eth

import (
    "bytes"
    "testing"

    cbg "github.com/whyrusleeping/cbor-gen"
    "github.com/stretchr/testify/require"
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

func TestCountAuthInDelegatorParams(t *testing.T) {
    // Build CBOR wrapper: [ list ] with list length 3
    var buf bytes.Buffer
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
    for i := 0; i < 3; i++ {
        // write an empty tuple placeholder (array(6) with zeroed children is fine)
        require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 0))
    }
    params := buf.Bytes()
    require.Equal(t, 3, countAuthInDelegatorParams(params))

    // Non-array should return 0
    buf.Reset()
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 7))
    require.Equal(t, 0, countAuthInDelegatorParams(buf.Bytes()))
}

func TestCountAuthInDelegatorParams_LegacyShape(t *testing.T) {
    // Build legacy top-level array with two tuples, where the first element is a tuple header (array(6)).
    // The counter should return the top-level length (2) when the first inner header is array(6).
    var buf bytes.Buffer
    // top-level array with length 2
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
    // first element: tuple header (array of 6)
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
    // we don't need to fully encode the tuple contents for counting
    require.Equal(t, 2, countAuthInDelegatorParams(buf.Bytes()))
}

func TestCountAuthInDelegatorParams_EmptyWrapper(t *testing.T) {
    var buf bytes.Buffer
    // wrapper [ list ] with list length 0
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 0))
    require.Equal(t, 0, countAuthInDelegatorParams(buf.Bytes()))
}
