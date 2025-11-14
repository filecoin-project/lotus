//go:build eip7702_enabled

package ethtypes

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

func TestEIP7702_ToUnsignedFilecoinMessage_EthAccountReceiver(t *testing.T) {
	// Configure an EthAccount.ApplyAndCall receiver directly and ensure the message targets it.
	id999, _ := address.NewIDAddress(999)
	EthAccountApplyAndCallActorAddr = id999

	var to EthAddress
	tx := &Eth7702TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Nonce:                0,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		AuthorizationList:    []EthAuthorization{{ChainID: EthUint64(buildconstants.Eip155ChainId), Address: to, Nonce: 0, YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))}},
		V:                    big.NewInt(0),
		R:                    big.NewInt(1),
		S:                    big.NewInt(1),
	}
	fromFC, err := (EthAddress{}).ToFilecoinAddress()
	require.NoError(t, err)
	msg, err := tx.ToUnsignedFilecoinMessage(fromFC)
	require.NoError(t, err)
	require.Equal(t, EthAccountApplyAndCallActorAddr, msg.To)
	require.EqualValues(t, MethodHash("ApplyAndCall"), msg.Method)
}
