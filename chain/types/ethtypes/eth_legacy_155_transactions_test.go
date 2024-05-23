package ethtypes

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
)

func TestEIP155Tx(t *testing.T) {
	txStr := "f86680843b9aca00835dc1ba94c2dca9a18d4a4057921d1bcb22da05e68e46b1d06480820297a0f91ee69c4603c4f21131467ee9e06ad4a96d0a29fa8064db61f3adaea0eb6e92a07181e306bb8f773d94cc3b75e9835de00c004e072e6630c0c46971d38706bb01"

	bz := mustDecodeHex(txStr)

	tx, err := parseLegacyTx(bz)
	require.NoError(t, err)

	eth155Tx, ok := tx.(*EthLegacy155TxArgs)
	require.True(t, ok)

	// Verify nonce
	require.EqualValues(t, 0, eth155Tx.legacyTx.Nonce)

	// Verify recipient address
	expectedToAddr, err := ParseEthAddress("0xc2dca9a18d4a4057921d1bcb22da05e68e46b1d0")
	require.NoError(t, err)
	require.EqualValues(t, expectedToAddr, *eth155Tx.legacyTx.To)

	// Verify sender address
	expectedFromAddr, err := ParseEthAddress("0xA2BBB73aC59b256415e91A820b224dbAF2C268FA")
	require.NoError(t, err)
	sender, err := eth155Tx.Sender()
	require.NoError(t, err)
	expectedFromFilecoinAddr, err := expectedFromAddr.ToFilecoinAddress()
	require.NoError(t, err)
	require.EqualValues(t, expectedFromFilecoinAddr, sender)

	// Verify transaction value
	expectedValue, ok := big.NewInt(0).SetString("100", 10)
	require.True(t, ok)
	require.True(t, eth155Tx.legacyTx.Value.Cmp(expectedValue) == 0)

	// Verify gas limit and gas price
	expectedGasPrice, ok := big.NewInt(0).SetString("1000000000", 10)
	require.True(t, ok)
	require.EqualValues(t, 6144442, eth155Tx.legacyTx.GasLimit)
	require.True(t, eth155Tx.legacyTx.GasPrice.Cmp(expectedGasPrice) == 0)

	require.Empty(t, eth155Tx.legacyTx.Input)

	// Verify signature values (v, r, s)
	expectedV, ok := big.NewInt(0).SetString("0297", 16)
	require.True(t, ok)
	require.True(t, eth155Tx.legacyTx.V.Cmp(expectedV) == 0)

	expectedR, ok := big.NewInt(0).SetString("f91ee69c4603c4f21131467ee9e06ad4a96d0a29fa8064db61f3adaea0eb6e92", 16)
	require.True(t, ok)
	require.True(t, eth155Tx.legacyTx.R.Cmp(expectedR) == 0)

	expectedS, ok := big.NewInt(0).SetString("7181e306bb8f773d94cc3b75e9835de00c004e072e6630c0c46971d38706bb01", 16)
	require.True(t, ok)
	require.True(t, eth155Tx.legacyTx.S.Cmp(expectedS) == 0)

	// Convert to signed Filecoin message and verify fields
	smsg, err := ToSignedFilecoinMessage(eth155Tx)
	require.NoError(t, err)

	require.EqualValues(t, smsg.Message.From, sender)

	expectedToFilecoinAddr, err := eth155Tx.legacyTx.To.ToFilecoinAddress()
	require.NoError(t, err)
	require.EqualValues(t, smsg.Message.To, expectedToFilecoinAddr)
	require.EqualValues(t, smsg.Message.Value, eth155Tx.legacyTx.Value)
	require.EqualValues(t, smsg.Message.GasLimit, eth155Tx.legacyTx.GasLimit)
	require.EqualValues(t, smsg.Message.GasFeeCap, eth155Tx.legacyTx.GasPrice)
	require.EqualValues(t, smsg.Message.GasPremium, eth155Tx.legacyTx.GasPrice)
	require.EqualValues(t, smsg.Message.Nonce, eth155Tx.legacyTx.Nonce)
	require.Empty(t, smsg.Message.Params)
	require.EqualValues(t, smsg.Message.Method, builtintypes.MethodsEVM.InvokeContract)

	// Convert signed Filecoin message back to Ethereum transaction and verify equality
	ethTx, err := EthTransactionFromSignedFilecoinMessage(smsg)
	require.NoError(t, err)
	convertedLegacyTx, ok := ethTx.(*EthLegacy155TxArgs)
	require.True(t, ok)
	eth155Tx.legacyTx.Input = nil
	require.EqualValues(t, convertedLegacyTx, eth155Tx)

	// Verify EthTx fields
	ethTxVal, err := eth155Tx.ToEthTx(smsg)
	require.NoError(t, err)
	expectedHash, err := eth155Tx.TxHash()
	require.NoError(t, err)
	require.EqualValues(t, ethTxVal.Hash, expectedHash)
	require.Nil(t, ethTxVal.MaxFeePerGas)
	require.Nil(t, ethTxVal.MaxPriorityFeePerGas)
	require.EqualValues(t, ethTxVal.Gas, eth155Tx.legacyTx.GasLimit)
	require.EqualValues(t, ethTxVal.Value, eth155Tx.legacyTx.Value)
	require.EqualValues(t, ethTxVal.Nonce, eth155Tx.legacyTx.Nonce)
	require.EqualValues(t, ethTxVal.To, eth155Tx.legacyTx.To)
	require.EqualValues(t, ethTxVal.From, expectedFromAddr)
}

func TestDeriveEIP155ChainId(t *testing.T) {
	tests := []struct {
		name            string
		v               big.Int
		expectedChainId big.Int
	}{
		{
			name:            "V equals 27",
			v:               big.NewInt(27),
			expectedChainId: big.NewInt(0),
		},
		{
			name:            "V equals 28",
			v:               big.NewInt(28),
			expectedChainId: big.NewInt(0),
		},
		{
			name:            "V small chain ID",
			v:               big.NewInt(37), // (37 - 35) / 2 = 1
			expectedChainId: big.NewInt(1),
		},
		{
			name:            "V large chain ID",
			v:               big.NewInt(1001), // (1001 - 35) / 2 = 483
			expectedChainId: big.NewInt(483),
		},
		{
			name:            "V very large chain ID",
			v:               big.NewInt(1 << 20), // (1048576 - 35) / 2 = 524770
			expectedChainId: big.NewInt(524270),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveEIP155ChainId(tt.v)
			require.True(t, result.Equals(tt.expectedChainId), "Expected %s, got %s for V=%s", tt.expectedChainId.String(), result.String(), tt.v.String())
		})
	}
}

func TestCalcEIP155TxSignatureLen(t *testing.T) {
	tests := []struct {
		name     string
		chainID  uint64
		expected int
	}{
		{
			name:     "ChainID that fits in 1 byte",
			chainID:  0x01,
			expected: EthLegacyHomesteadTxSignatureLen + 1 - 1,
		},
		{
			name:     "ChainID that fits in 2 bytes",
			chainID:  0x0100,
			expected: EthLegacyHomesteadTxSignatureLen + 2 - 1,
		},
		{
			name:     "ChainID that fits in 3 bytes",
			chainID:  0x010000,
			expected: EthLegacyHomesteadTxSignatureLen + 3 - 1,
		},
		{
			name:     "ChainID that fits in 4 bytes",
			chainID:  0x01000000,
			expected: EthLegacyHomesteadTxSignatureLen + 4 - 1,
		},
		{
			name:     "ChainID that fits in 6 bytes",
			chainID:  0x010000000000,
			expected: EthLegacyHomesteadTxSignatureLen + 6 - 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcEIP155TxSignatureLen(tt.chainID, 1)
			if result != tt.expected {
				t.Errorf("calcEIP155TxSignatureLen(%d) = %d, want %d", tt.chainID, result, tt.expected)
			}
		})
	}
}
