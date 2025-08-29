package eth

import (
    "bytes"
    "testing"

    cbg "github.com/whyrusleeping/cbor-gen"
    "github.com/stretchr/testify/require"
)

func TestCompute7702IntrinsicOverhead(t *testing.T) {
    require.EqualValues(t, 0, compute7702IntrinsicOverhead(0))
    // base (2100) + 2 * 25000
    require.EqualValues(t, 52100, compute7702IntrinsicOverhead(2))
}

func TestCountAuthInDelegatorParams(t *testing.T) {
    // Build CBOR: array of 3 tuples [] (we only care about outer array length)
    var buf bytes.Buffer
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
    for i := 0; i < 3; i++ {
        // write an empty inner array header (shape is irrelevant for counter)
        require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 0))
    }
    params := buf.Bytes()
    require.Equal(t, 3, countAuthInDelegatorParams(params))

    // Non-array should return 0
    buf.Reset()
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 7))
    require.Equal(t, 0, countAuthInDelegatorParams(buf.Bytes()))
}

