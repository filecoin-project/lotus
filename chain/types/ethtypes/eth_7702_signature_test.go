package ethtypes

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"
)

func TestEIP7702_SignaturePacking(t *testing.T) {
	var to EthAddress
	tx := &Eth7702TxArgs{
		ChainID:              1,
		Nonce:                0,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		AuthorizationList:    []EthAuthorization{{ChainID: 1, Address: to, Nonce: 0, YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))}},
		V:                    big.NewInt(1),
		R:                    big.NewInt(1),
		S:                    big.NewInt(2),
	}
	sig, err := tx.Signature()
	require.NoError(t, err)
	require.Equal(t, typescrypto.SigTypeDelegated, sig.Type)
	require.Len(t, sig.Data, 65)
	// r at index 31 must be 0x01; s at index 63 must be 0x02; v at index 64 must be 0x01
	require.EqualValues(t, 0x01, sig.Data[31])
	require.EqualValues(t, 0x02, sig.Data[63])
	require.EqualValues(t, 0x01, sig.Data[64])

	// ToVerifiableSignature is a no-op for v-parity signatures
	vsig, err := tx.ToVerifiableSignature(sig.Data)
	require.NoError(t, err)
	require.Equal(t, sig.Data, vsig)
}
