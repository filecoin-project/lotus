package delegator

import (
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/filecoin-project/go-address"
    ethtypes "github.com/filecoin-project/lotus/chain/types/ethtypes"
    stbig "github.com/filecoin-project/go-state-types/big"
)

func TestApplyDelegationsCore_AppliesAndBumpsNonce(t *testing.T) {
    // Build one authorization tuple via ethtypes to ensure CBOR compatibility.
    var authAddr ethtypes.EthAddress
    for i := range authAddr { authAddr[i] = 0x33 }
    list := []ethtypes.EthAuthorization{{
        ChainID: 314,
        Address: authAddr,
        Nonce:   10,
        YParity: 1,
        R:       ethtypes.EthBigInt(stbig.NewInt(1)),
        S:       ethtypes.EthBigInt(stbig.NewInt(1)),
    }}
    enc, err := ethtypes.CborEncodeEIP7702Authorizations(list)
    require.NoError(t, err)

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
