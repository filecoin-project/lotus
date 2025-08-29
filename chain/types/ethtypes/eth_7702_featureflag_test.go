//go:build eip7702_enabled

package ethtypes

import (
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/filecoin-project/go-address"
    "github.com/filecoin-project/go-state-types/big"
    "github.com/filecoin-project/lotus/build/buildconstants"
    delegator "github.com/filecoin-project/lotus/chain/actors/builtin/delegator"
)

// Validates that when the eip7702_enabled build tag is set and the DelegatorActorAddr
// is configured, ToUnsignedFilecoinMessage constructs a message targeting the Delegator
// actor with CBOR-encoded params.
func Test7702_ToUnsignedFilecoinMessage_FeatureFlag(t *testing.T) {
    // Configure a fake Delegator actor address
    a, err := address.NewIDAddress(1234)
    require.NoError(t, err)
    DelegatorActorAddr = a

    // Minimal 7702 tx with one authorization
    var to EthAddress
    copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
    tx := &Eth7702TxArgs{
        ChainID:              buildconstants.Eip155ChainId,
        Nonce:                1,
        To:                   &to,
        Value:                big.NewInt(0),
        MaxFeePerGas:         big.NewInt(1),
        MaxPriorityFeePerGas: big.NewInt(1),
        GasLimit:             21000,
        AuthorizationList: []EthAuthorization{
            {ChainID: EthUint64(buildconstants.Eip155ChainId), Address: to, Nonce: EthUint64(1), YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))},
        },
        V: big.NewInt(0), R: big.NewInt(1), S: big.NewInt(1),
    }

    from, err := address.NewIDAddress(999)
    require.NoError(t, err)
    msg, err := tx.ToUnsignedFilecoinMessage(from)
    require.NoError(t, err)
    require.Equal(t, DelegatorActorAddr, msg.To)
    require.EqualValues(t, delegator.MethodApplyDelegations, msg.Method)
    require.NotEmpty(t, msg.Params)
}
