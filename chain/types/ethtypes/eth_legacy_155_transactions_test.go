package ethtypes

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/lib/sigs"
)

func TestEthLegacy155TxArgs(t *testing.T) {
	testcases := []struct {
		RawTx            string
		ExpectedNonce    uint64
		ExpectedTo       string
		ExpectedInput    string
		ExpectedGasPrice big.Int
		ExpectedGasLimit int

		ExpectErr bool
	}{
		{
			"0xf8708310aa048504a817c80083015f9094f8c911c68f6a6b912fe735bbd953c3379336cbf3880e19fb7ff12c8c308025a09abb6d2bb66c9520f76391169e1155e8426e41ce9888e16d93512083038de023a020bbd446f8dcbdd996a5e1d0663ec8e28c33d5cb254e323855331be71bc8b0fb",
			0x0,
			"0x095e7baea6a6c7c4c2dfeb977efac326af552d87",
			"0xdeadbeef0000000101010010101010101010101010101aaabbbbbbcccccccddddddddd",
			big.NewInt(1),
			0x5408,
			false,
		},
		/*{
			"0xf85f030182520794b94f5374fce5edbc8e2a8697c15331677e6ebf0b0a801ba098ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4aa07778cde41a8a37f6a087622b38bc201bd3e7df06dce067569d4def1b53dba98c",
			0x3,
			"0xb94f5374fce5edbc8e2a8697c15331677e6ebf0b",
			"0x",
			big.NewInt(1),
			0x5207,
			false,
		},
		*/
	}

	for i, tc := range testcases {
		// parse txargs
		tx, err := parseLegacyTx(mustDecodeHex(tc.RawTx))
		require.NoError(t, err)

		msgRecovered, err := tx.ToRlpUnsignedMsg()
		require.NoError(t, err)

		// verify signatures
		from, err := tx.Sender()
		require.NoError(t, err)

		smsg, err := ToSignedFilecoinMessage(tx)
		require.NoError(t, err)

		sig := smsg.Signature.Data[:]
		sig = sig[1:]
		vValue := big.NewInt(0).SetBytes(sig[64:])
		vValue_ := big.Sub(big.NewFromGo(vValue), big.NewInt(27))
		sig[64] = byte(vValue_.Uint64())
		smsg.Signature.Data = sig

		err = sigs.Verify(&smsg.Signature, from, msgRecovered)
		require.NoError(t, err)

		txArgs := tx.(*EthLegacyHomesteadTxArgs)
		// verify data
		require.EqualValues(t, tc.ExpectedNonce, txArgs.Nonce, i)

		expectedTo, err := ParseEthAddress(tc.ExpectedTo)
		require.NoError(t, err)
		require.EqualValues(t, expectedTo, *txArgs.To, i)
		require.EqualValues(t, tc.ExpectedInput, "0x"+hex.EncodeToString(txArgs.Input))
		require.EqualValues(t, tc.ExpectedGasPrice, txArgs.GasPrice)
		require.EqualValues(t, tc.ExpectedGasLimit, txArgs.GasLimit)
	}
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
			assert.True(t, result.Equals(tt.expectedChainId), "Expected %s, got %s for V=%s", tt.expectedChainId.String(), result.String(), tt.v.String())
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
			result := calcEIP155TxSignatureLen(tt.chainID)
			if result != tt.expected {
				t.Errorf("calcEIP155TxSignatureLen(%d) = %d, want %d", tt.chainID, result, tt.expected)
			}
		})
	}
}
