package ethtypes

import (
    "bytes"
    "testing"

    cbg "github.com/whyrusleeping/cbor-gen"
    "github.com/stretchr/testify/require"

    "github.com/filecoin-project/go-address"
    builtintypes "github.com/filecoin-project/go-state-types/builtin"
    typescrypto "github.com/filecoin-project/go-state-types/crypto"
    "github.com/filecoin-project/lotus/chain/actors/builtin/delegator"
    "github.com/filecoin-project/lotus/chain/types"
)

// encodeAuthWrapper encodes a wrapper [ list ] with one 6-tuple for convenience.
func encodeAuthWrapper(t *testing.T) []byte {
    t.Helper()
    var buf bytes.Buffer
    // wrapper [ list ]
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
    // tuple [ chain_id, address(20), nonce, y_parity, r, s ]
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 314))
    // 20-byte address
    var addr [20]byte
    for i := range addr { addr[i] = 0xaa }
    require.NoError(t, cbg.WriteByteArray(&buf, addr[:]))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0)) // nonce
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0)) // y_parity
    require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))              // r
    require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))              // s
    return buf.Bytes()
}

func TestEthTransactionFromSignedMessage_7702_Decodes(t *testing.T) {
    // Setup: set DelegatorActorAddr to ID:18
    id18, _ := address.NewIDAddress(18)
    DelegatorActorAddr = id18
    // From must be an eth (f4) address
    var from20 [20]byte
    for i := range from20 { from20[i] = 0x11 }
    from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
    require.NoError(t, err)

    // Build SignedMessage targeting Delegator.ApplyDelegations
    msg := types.Message{
        Version:    0,
        To:         DelegatorActorAddr,
        From:       from,
        Nonce:      0,
        Value:      types.NewInt(0),
        Method:     delegator.MethodApplyDelegations,
        GasLimit:   100000,
        GasFeeCap:  types.NewInt(1),
        GasPremium: types.NewInt(1),
        Params:     encodeAuthWrapper(t),
    }
    // Fake a delegated 65-byte signature r||s||v where r,s=1 and v=0
    sig := typescrypto.Signature{ Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
    smsg := &types.SignedMessage{ Message: msg, Signature: sig }

    tx, err := EthTransactionFromSignedFilecoinMessage(smsg)
    require.NoError(t, err)
    // Expect a 0x04 typed tx and authorizationList echoed
    require.EqualValues(t, EIP7702TxType, tx.Type())
    eth, err := tx.ToEthTx(smsg)
    require.NoError(t, err)
    require.Len(t, eth.AuthorizationList, 1)
    require.EqualValues(t, 314, eth.AuthorizationList[0].ChainID)
}
