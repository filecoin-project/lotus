package ethtypes

import (
    "encoding/hex"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/filecoin-project/go-state-types/big"
)

func mustHex(t *testing.T, s string) []byte {
    t.Helper()
    s = remove0x(s)
    b, err := hex.DecodeString(s)
    require.NoError(t, err)
    return b
}

func remove0x(s string) string {
    if len(s) >= 2 && (s[0:2] == "0x" || s[0:2] == "0X") {
        return s[2:]
    }
    return s
}

func TestEIP7702_RLPRoundTrip(t *testing.T) {
    // Build a small, valid-looking EIP-7702 transaction with one authorization tuple.
    var to EthAddress
    copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))

    var authAddr EthAddress
    copy(authAddr[:], mustHex(t, "0x2222222222222222222222222222222222222222"))

    tx := &Eth7702TxArgs{
        ChainID:              1,
        Nonce:                5,
        To:                   &to,
        Value:                big.NewInt(0),
        MaxFeePerGas:         big.NewInt(1000000000),
        MaxPriorityFeePerGas: big.NewInt(100000000),
        GasLimit:             21000,
        Input:                mustHex(t, "0xdeadbeef"),
        AuthorizationList: []EthAuthorization{
            {
                ChainID: EthUint64(1),
                Address: authAddr,
                Nonce:   EthUint64(7),
                YParity: 0,
                R:       EthBigInt(big.NewInt(1)),
                S:       EthBigInt(big.NewInt(2)),
            },
        },
        V: big.NewInt(1),
        R: big.NewInt(3),
        S: big.NewInt(4),
    }

    // Encode to signed RLP (includes type 0x04 prefix)
    enc, err := tx.ToRlpSignedMsg()
    require.NoError(t, err)
    require.Greater(t, len(enc), 1)
    require.Equal(t, byte(EIP7702TxType), enc[0])

    // Parse back
    dec, err := parseEip7702Tx(enc)
    require.NoError(t, err)

    // Spot-check fields
    require.Equal(t, tx.ChainID, dec.ChainID)
    require.Equal(t, tx.Nonce, dec.Nonce)
    require.Equal(t, tx.GasLimit, dec.GasLimit)
    require.Equal(t, tx.To, dec.To)
    require.True(t, tx.Value.Equals(dec.Value))
    require.Equal(t, tx.Input, dec.Input)
    require.True(t, tx.MaxFeePerGas.Equals(dec.MaxFeePerGas))
    require.True(t, tx.MaxPriorityFeePerGas.Equals(dec.MaxPriorityFeePerGas))
    require.Equal(t, 1, len(dec.AuthorizationList))
    require.Equal(t, tx.AuthorizationList[0].ChainID, dec.AuthorizationList[0].ChainID)
    require.Equal(t, tx.AuthorizationList[0].Address, dec.AuthorizationList[0].Address)
    require.Equal(t, tx.AuthorizationList[0].Nonce, dec.AuthorizationList[0].Nonce)
    require.Equal(t, tx.AuthorizationList[0].YParity, dec.AuthorizationList[0].YParity)
    require.Equal(t, tx.AuthorizationList[0].R.String(), dec.AuthorizationList[0].R.String())
    require.Equal(t, tx.AuthorizationList[0].S.String(), dec.AuthorizationList[0].S.String())
    require.Equal(t, tx.V.String(), dec.V.String())
    require.Equal(t, tx.R.String(), dec.R.String())
    require.Equal(t, tx.S.String(), dec.S.String())

    // Re-encode parsed tx and compare bytes exactly
    enc2, err := dec.ToRlpSignedMsg()
    require.NoError(t, err)
    require.Equal(t, enc, enc2)
}

