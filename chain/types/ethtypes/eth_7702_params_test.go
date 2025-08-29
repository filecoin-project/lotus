package ethtypes

import (
    "bytes"
    "testing"

    cbg "github.com/whyrusleeping/cbor-gen"
    "github.com/stretchr/testify/require"
    "github.com/filecoin-project/go-state-types/big"
)

func TestCborEncodeEIP7702Authorizations_Shape(t *testing.T) {
    var addr1, addr2 EthAddress
    copy(addr1[:], mustHex(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
    copy(addr2[:], mustHex(t, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))

    list := []EthAuthorization{
        {ChainID: 1, Address: addr1, Nonce: 7, YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(2))},
        {ChainID: 1, Address: addr2, Nonce: 8, YParity: 1, R: EthBigInt(big.NewInt(3)), S: EthBigInt(big.NewInt(4))},
    }

    enc, err := CborEncodeEIP7702Authorizations(list)
    require.NoError(t, err)
    require.NotEmpty(t, enc)

    r := cbg.NewCborReader(bytes.NewReader(enc))
    maj, l, err := r.ReadHeader()
    require.NoError(t, err)
    require.Equal(t, byte(cbg.MajArray), maj)
    require.Equal(t, uint64(2), l)

    for i := 0; i < 2; i++ {
        maj, l, err = r.ReadHeader()
        require.NoError(t, err)
        require.Equal(t, byte(cbg.MajArray), maj)
        require.Equal(t, uint64(6), l)

        // chain_id
        maj, v, err := r.ReadHeader()
        require.NoError(t, err)
        require.Equal(t, byte(cbg.MajUnsignedInt), maj)
        require.Equal(t, uint64(1), v)
        // address bytes
        maj, v, err = r.ReadHeader()
        require.NoError(t, err)
        require.Equal(t, byte(cbg.MajByteString), maj)
        require.Equal(t, uint64(20), v)
        // skip exact bytes
        buf := make([]byte, v)
        _, err = r.Read(buf)
        require.NoError(t, err)
        // nonce
        maj, v, err = r.ReadHeader()
        require.NoError(t, err)
        require.Equal(t, byte(cbg.MajUnsignedInt), maj)
        // y_parity
        maj, v, err = r.ReadHeader()
        require.NoError(t, err)
        require.Equal(t, byte(cbg.MajUnsignedInt), maj)
        require.True(t, v == 0 || v == 1)
        // r bytes
        maj, v, err = r.ReadHeader()
        require.NoError(t, err)
        require.Equal(t, byte(cbg.MajByteString), maj)
        _, err = r.Read(make([]byte, v))
        require.NoError(t, err)
        // s bytes
        maj, v, err = r.ReadHeader()
        require.NoError(t, err)
        require.Equal(t, byte(cbg.MajByteString), maj)
        _, err = r.Read(make([]byte, v))
        require.NoError(t, err)
    }
}
