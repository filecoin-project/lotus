package delegator

import (
    "bytes"
    "testing"

    cbg "github.com/whyrusleeping/cbor-gen"
    "github.com/stretchr/testify/require"
    mathbig "math/big"
    "github.com/filecoin-project/go-address"
)

// helper to encode tuples inline (mirror ethtypes encoder, no import cycles)
func encodeTuples(t *testing.T, tuples [][]interface{}) []byte {
    t.Helper()
    var buf bytes.Buffer
    // Write wrapper [ list ]
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, uint64(len(tuples))))
    for _, tup := range tuples {
        require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, uint64(len(tup))))
        // chain_id
        require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, tup[0].(uint64)))
        // address (20 bytes)
        require.NoError(t, cbg.WriteByteArray(&buf, tup[1].([]byte)))
        // nonce
        require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, tup[2].(uint64)))
        // y_parity
        require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, tup[3].(uint64)))
        // r
        require.NoError(t, cbg.WriteByteArray(&buf, tup[4].([]byte)))
        // s
        require.NoError(t, cbg.WriteByteArray(&buf, tup[5].([]byte)))
    }
    return buf.Bytes()
}

func TestDecodeAndValidateDelegations_OK(t *testing.T) {
    addr1 := make([]byte, 20)
    for i := range addr1 { addr1[i] = 0xaa }
    addr2 := make([]byte, 20)
    for i := range addr2 { addr2[i] = 0xbb }
    tuples := [][]interface{}{
        {uint64(314), addr1, uint64(7), uint64(0), []byte{1}, []byte{1}},
        {uint64(314), addr2, uint64(8), uint64(1), []byte{2}, []byte{2}},
    }
    enc := encodeTuples(t, tuples)
    list, err := DecodeAuthorizationTuples(enc)
    require.NoError(t, err)
    require.Len(t, list, 2)
    require.NoError(t, ValidateDelegations(list, 314))
}

func TestApplyDelegationsWithAuthorities_WritesAndBumpsNonce(t *testing.T) {
    addr1 := make([]byte, 20)
    for i := range addr1 { addr1[i] = 0xcc }
    tuples := [][]interface{}{
        {uint64(314), addr1, uint64(5), uint64(0), []byte{1}, []byte{1}},
    }
    enc := encodeTuples(t, tuples)
    list, err := DecodeAuthorizationTuples(enc)
    require.NoError(t, err)
    require.NoError(t, ValidateDelegations(list, 314))

    // Prepare state and authority nonce
    var st State
    auth, err := address.NewIDAddress(1001)
    require.NoError(t, err)
    nonces := map[address.Address]uint64{auth: 5}
    authorities := []address.Address{auth}

    // Apply
    err = st.ApplyDelegationsWithAuthorities(nonces, authorities, list)
    require.NoError(t, err)

    // Mapping set and nonce incremented
    v, ok := st.Delegations[auth]
    require.True(t, ok)
    require.Equal(t, [20]byte{0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc,0xcc}, v)
    require.EqualValues(t, 6, nonces[auth])
}

func TestApplyDelegationsWithAuthorities_NonceMismatch(t *testing.T) {
    addr1 := make([]byte, 20)
    tuples := [][]interface{}{
        {uint64(314), addr1, uint64(9), uint64(0), []byte{1}, []byte{1}},
    }
    enc := encodeTuples(t, tuples)
    list, err := DecodeAuthorizationTuples(enc)
    require.NoError(t, err)
    require.NoError(t, ValidateDelegations(list, 314))

    var st State
    auth, err := address.NewIDAddress(1001)
    require.NoError(t, err)
    nonces := map[address.Address]uint64{auth: 8}
    authorities := []address.Address{auth}

    err = st.ApplyDelegationsWithAuthorities(nonces, authorities, list)
    require.Error(t, err)
}

// Cross-package CBOR compat test moved to ethtypes package to avoid import cycles.

func TestDecodeAndValidateDelegations_InvalidYParity(t *testing.T) {
    addr := make([]byte, 20)
    tuples := [][]interface{}{
        {uint64(314), addr, uint64(0), uint64(2), []byte{1}, []byte{1}},
    }
    enc := encodeTuples(t, tuples)
    list, err := DecodeAuthorizationTuples(enc)
    require.NoError(t, err)
    require.Len(t, list, 1)
    require.Error(t, ValidateDelegations(list, 314))
}

func TestDecodeAndValidateDelegations_HighSRejected(t *testing.T) {
    addr := make([]byte, 20)
    // s = halforder + 1 (just construct by adding 1 to the hex constant)
    highS := new(secbig).Add(secp256k1HalfOrder, one())
    sbytes := highS.Bytes()
    tuples := [][]interface{}{
        {uint64(314), addr, uint64(0), uint64(0), []byte{1}, sbytes},
    }
    enc := encodeTuples(t, tuples)
    list, err := DecodeAuthorizationTuples(enc)
    require.NoError(t, err)
    require.Error(t, ValidateDelegations(list, 314))
}

func TestApplyDelegationsFromCBOR_ValidatesAndReturnsList(t *testing.T) {
    addr := make([]byte, 20)
    tuples := [][]interface{}{
        {uint64(314), addr, uint64(9), uint64(1), []byte{3}, []byte{4}},
    }
    enc := encodeTuples(t, tuples)
    list, err := ApplyDelegationsFromCBOR(enc, 314)
    require.NoError(t, err)
    require.Len(t, list, 1)
    require.EqualValues(t, 314, list[0].ChainID)
    require.EqualValues(t, 9, list[0].Nonce)
    require.EqualValues(t, 1, list[0].YParity)
}

// small helpers to avoid math/big import aliasing in test
type secbig = mathbig.Int
func one() *secbig { return new(secbig).SetUint64(1) }

func TestDecodeAuthorizationTuples_WrapperFormAccepted(t *testing.T) {
    // Build wrapper [ list ] where list contains one valid tuple
    var buf bytes.Buffer
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
    // chain_id
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 314))
    // address
    addr := make([]byte, 20)
    for i := range addr { addr[i] = 0x44 }
    require.NoError(t, cbg.WriteByteArray(&buf, addr))
    // nonce, y_parity
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 1))
    // r, s
    require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
    require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))

    tuples, err := DecodeAuthorizationTuples(buf.Bytes())
    require.NoError(t, err)
    require.Len(t, tuples, 1)
    require.NoError(t, ValidateDelegations(tuples, 314))
}

func TestDecodeAuthorizationTuples_TopLevelNotArray(t *testing.T) {
    var buf bytes.Buffer
    // Write an unsigned int instead of array
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 7))
    _, err := DecodeAuthorizationTuples(buf.Bytes())
    require.Error(t, err)
}
