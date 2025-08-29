package ethtypes

import (
    "testing"
    "github.com/stretchr/testify/require"
    delegator "github.com/filecoin-project/lotus/chain/actors/builtin/delegator"
    stbig "github.com/filecoin-project/go-state-types/big"
)

// Ensures delegator decodes CBOR encoded by ethtypes encoder; avoids import cycles
// by placing the cross-package test here (ethtypes already imports delegator).
func Test7702_CrossPackage_CborCompat(t *testing.T) {
    var a1, a2 EthAddress
    for i := range a1 { a1[i] = 0x11 }
    for i := range a2 { a2[i] = 0x22 }
    list := []EthAuthorization{
        { ChainID: 314, Address: a1, Nonce: 1, YParity: 0, R: EthBigInt(stbig.NewInt(1)), S: EthBigInt(stbig.NewInt(2)) },
        { ChainID: 314, Address: a2, Nonce: 2, YParity: 1, R: EthBigInt(stbig.NewInt(3)), S: EthBigInt(stbig.NewInt(4)) },
    }
    enc, err := CborEncodeEIP7702Authorizations(list)
    require.NoError(t, err)

    dl, err := delegator.DecodeAuthorizationTuples(enc)
    require.NoError(t, err)
    require.Len(t, dl, 2)
    require.NoError(t, delegator.ValidateDelegations(dl, 314))

    require.EqualValues(t, 314, dl[0].ChainID)
    require.EqualValues(t, 1, dl[0].Nonce)
    require.EqualValues(t, 0, dl[0].YParity)
    require.EqualValues(t, 1, dl[0].R.Int64())
    require.EqualValues(t, 2, dl[0].S.Int64())
}

