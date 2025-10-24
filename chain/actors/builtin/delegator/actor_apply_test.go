package delegator

import (
    "bytes"
    "testing"

    cbg "github.com/whyrusleeping/cbor-gen"
    "github.com/stretchr/testify/require"
    "github.com/filecoin-project/go-address"
)

func TestApplyDelegationsCore_AppliesAndBumpsNonce(t *testing.T) {
    // Build one authorization tuple CBOR directly.
    var buf bytes.Buffer
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1)) // wrapper
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1)) // inner list length 1
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6)) // tuple
    // chainId
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 314))
    // address (20 bytes of 0x33)
    addr := make([]byte, 20)
    for i := range addr { addr[i] = 0x33 }
    require.NoError(t, cbg.WriteByteArray(&buf, addr))
    // nonce
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 10))
    // y_parity
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 1))
    // r
    require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
    // s
    require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
    enc := buf.Bytes()

    // Prepare state, nonces, and authorities (pre-resolved for this scaffold test).
    var st State
    auth, err := address.NewIDAddress(777)
    require.NoError(t, err)
    nonces := map[address.Address]uint64{auth: 10}
    authorities := []address.Address{auth}

    // Apply
    require.NoError(t, ApplyDelegationsCore(&st, nonces, authorities, enc, 314))

    // Mapping is set and nonce bumped
    v, ok := st.Delegations[auth]
    require.True(t, ok)
    require.Equal(t, [20]byte{0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33,0x33}, v)
    require.EqualValues(t, 11, nonces[auth])
}
