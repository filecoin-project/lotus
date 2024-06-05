package ethtypes

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/lib/sigs"
)

func TestEthLegacyHomesteadTxArgs(t *testing.T) {
	testcases := []struct {
		RawTx            string
		ExpectedNonce    uint64
		ExpectedTo       string
		ExpectedInput    string
		ExpectedGasPrice big.Int
		ExpectedGasLimit int
		ExpectErr        bool
	}{
		{
			"0xf882800182540894095e7baea6a6c7c4c2dfeb977efac326af552d8780a3deadbeef0000000101010010101010101010101010101aaabbbbbbcccccccddddddddd1ba048b55bfa915ac795c431978d8a6a992b628d557da5ff759b307d495a36649353a01fffd310ac743f371de3b9f7f9cb56c0b28ad43601b4ab949f53faa07bd2c804",
			0x0,
			"0x095e7baea6a6c7c4c2dfeb977efac326af552d87",
			"0xdeadbeef0000000101010010101010101010101010101aaabbbbbbcccccccddddddddd",
			big.NewInt(1),
			0x5408,
			false,
		},
		{
			"0xf85f030182520794b94f5374fce5edbc8e2a8697c15331677e6ebf0b0a801ba098ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4aa07778cde41a8a37f6a087622b38bc201bd3e7df06dce067569d4def1b53dba98c",
			0x3,
			"0xb94f5374fce5edbc8e2a8697c15331677e6ebf0b",
			"0x",
			big.NewInt(1),
			0x5207,
			false,
		},
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

func TestLegacyHomesteadSignatures(t *testing.T) {
	testcases := []struct {
		RawTx           string
		ExpectedR       string
		ExpectedS       string
		ExpectedV       string
		ExpectErr       bool
		ExpectErrMsg    string
		ExpectVMismatch bool
	}{
		{
			"0xf882800182540894095e7baea6a6c7c4c2dfeb977efac326af552d8780a3deadbeef0000000101010010101010101010101010101aaabbbbbbcccccccddddddddd1ba048b55bfa915ac795c431978d8a6a992b628d557da5ff759b307d495a36649353a01fffd310ac743f371de3b9f7f9cb56c0b28ad43601b4ab949f53faa07bd2c804",
			"0x48b55bfa915ac795c431978d8a6a992b628d557da5ff759b307d495a36649353",
			"0x1fffd310ac743f371de3b9f7f9cb56c0b28ad43601b4ab949f53faa07bd2c804",
			"0x1b",
			false,
			"",
			false,
		},
		{
			"0xf85f030182520794b94f5374fce5edbc8e2a8697c15331677e6ebf0b0a801ba098ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4aa07778cde41a8a37f6a087622b38bc201bd3e7df06dce067569d4def1b53dba98c",
			"0x98ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4a",
			"0x7778cde41a8a37f6a087622b38bc201bd3e7df06dce067569d4def1b53dba98c",
			"0x1b",
			false,
			"",
			false,
		},
		{
			"0xf882800182540894095e7baea6a6c7c4c2dfeb977efac326af552d8780a3deadbeef0000000101010010101010101010101010101aaabbbbbbcccccccddddddddd1ba048b55bfa915ac795c431978d8a6a992b628d557da5ff759b307d495a36649353a01fffd310ac743f371de3b9f7f9cb56c0b28ad43601b4ab949f53faa07bd2c804",
			"0x48b55bfa915ac795c431978d8a6a992b628d557da5ff759b307d495a36649353",
			"0x1fffd310ac743f371de3b9f7f9cb56c0b28ad43601b4ab949f53faa07bd2c804",
			"0x1c",
			false,
			"",
			true,
		},
		{
			"0xf882800182540894095e7baea6a6c7c4c2dfeb977efac326af552d8780a3deadbeef0000000101010010101010101010101010101aaabbbbbbcccccccddddddddd1ba048b55bfa915ac795c431978d8a6a992b628d557da5ff759b307d495a36649353a01fffd310ac743f371de3b9f7f9cb56c0b28ad43601b4ab949f53faa07bd2c804",
			"0x48b55bfa915ac795c431978d8a6a992b628d557da5ff759b307d495a36649353",
			"0x1fffd310ac743f371de3b9f7f9cb56c0b28ad43601b4ab949f53faa07bd2c804",
			"0x1f",
			false,
			"",
			true,
		},
		{
			"0x00",
			"",
			"",
			"",
			true,
			"not a legacy eth transaction",
			false,
		},
	}

	for i, tc := range testcases {
		tx, err := parseLegacyTx(mustDecodeHex(tc.RawTx))
		if tc.ExpectErr {
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.ExpectErrMsg)
			continue
		}
		require.Nil(t, err)

		sig, err := tx.Signature()
		require.Nil(t, err)

		require.NoError(t, tx.InitialiseSignature(*sig))

		txArgs := tx.(*EthLegacyHomesteadTxArgs)

		require.Equal(t, tc.ExpectedR, "0x"+txArgs.R.Text(16), i)
		require.Equal(t, tc.ExpectedS, "0x"+txArgs.S.Text(16), i)

		if tc.ExpectVMismatch {
			require.NotEqual(t, tc.ExpectedV, "0x"+txArgs.V.Text(16), i)
		} else {
			require.Equal(t, tc.ExpectedV, "0x"+txArgs.V.Text(16), i)
		}
	}
}

// https://etherscan.io/getRawTx?tx=0xc55e2b90168af6972193c1f86fa4d7d7b31a29c156665d15b9cd48618b5177ef
// https://tools.deth.net/tx-decoder
func TestEtherScanLegacyRLP(t *testing.T) {
	rlp := "0xf8718301efc58506fc23ac008305161594104994f45d9d697ca104e5704a7b77d7fec3537c890821878651a4d70000801ba051222d91a379452395d0abaff981af4cfcc242f25cfaf947dea8245a477731f9a03a997c910b4701cca5d933fb26064ee5af7fe3236ff0ef2b58aa50b25aff8ca5"
	bz := mustDecodeHex(rlp)

	tx, err := parseLegacyTx(bz)
	require.NoError(t, err)

	ethLegacyTx, ok := tx.(*EthLegacyHomesteadTxArgs)
	require.True(t, ok)

	// Verify nonce
	require.EqualValues(t, 0x1efc5, ethLegacyTx.Nonce)

	// Verify recipient address
	expectedToAddr, err := ParseEthAddress("0x104994f45d9d697ca104e5704a7b77d7fec3537c")
	require.NoError(t, err)
	require.EqualValues(t, expectedToAddr, *ethLegacyTx.To)

	// Verify sender address
	expectedFromAddr, err := ParseEthAddress("0x32Be343B94f860124dC4fEe278FDCBD38C102D88")
	require.NoError(t, err)
	sender, err := ethLegacyTx.Sender()
	require.NoError(t, err)
	expectedFromFilecoinAddr, err := expectedFromAddr.ToFilecoinAddress()
	require.NoError(t, err)
	require.EqualValues(t, expectedFromFilecoinAddr, sender)

	// Verify transaction value
	expectedValue, ok := big.NewInt(0).SetString("821878651a4d70000", 16)
	require.True(t, ok)
	require.True(t, ethLegacyTx.Value.Cmp(expectedValue) == 0)

	// Verify gas limit and gas price
	expectedGasPrice, ok := big.NewInt(0).SetString("6fc23ac00", 16)
	require.True(t, ok)
	require.EqualValues(t, 0x51615, ethLegacyTx.GasLimit)
	require.True(t, ethLegacyTx.GasPrice.Cmp(expectedGasPrice) == 0)

	require.Empty(t, ethLegacyTx.Input)

	// Verify signature values (v, r, s)
	expectedV, ok := big.NewInt(0).SetString("1b", 16)
	require.True(t, ok)
	require.True(t, ethLegacyTx.V.Cmp(expectedV) == 0)

	expectedR, ok := big.NewInt(0).SetString("51222d91a379452395d0abaff981af4cfcc242f25cfaf947dea8245a477731f9", 16)
	require.True(t, ok)
	require.True(t, ethLegacyTx.R.Cmp(expectedR) == 0)

	expectedS, ok := big.NewInt(0).SetString("3a997c910b4701cca5d933fb26064ee5af7fe3236ff0ef2b58aa50b25aff8ca5", 16)
	require.True(t, ok)
	require.True(t, ethLegacyTx.S.Cmp(expectedS) == 0)

	// Convert to signed Filecoin message and verify fields
	smsg, err := ToSignedFilecoinMessage(ethLegacyTx)
	require.NoError(t, err)

	require.EqualValues(t, smsg.Message.From, sender)

	expectedToFilecoinAddr, err := ethLegacyTx.To.ToFilecoinAddress()
	require.NoError(t, err)
	require.EqualValues(t, smsg.Message.To, expectedToFilecoinAddr)
	require.EqualValues(t, smsg.Message.Value, ethLegacyTx.Value)
	require.EqualValues(t, smsg.Message.GasLimit, ethLegacyTx.GasLimit)
	require.EqualValues(t, smsg.Message.GasFeeCap, ethLegacyTx.GasPrice)
	require.EqualValues(t, smsg.Message.GasPremium, ethLegacyTx.GasPrice)
	require.EqualValues(t, smsg.Message.Nonce, ethLegacyTx.Nonce)
	require.Empty(t, smsg.Message.Params)
	require.EqualValues(t, smsg.Message.Method, builtintypes.MethodsEVM.InvokeContract)

	// Convert signed Filecoin message back to Ethereum transaction and verify equality
	ethTx, err := EthTransactionFromSignedFilecoinMessage(smsg)
	require.NoError(t, err)
	convertedLegacyTx, ok := ethTx.(*EthLegacyHomesteadTxArgs)
	require.True(t, ok)
	ethLegacyTx.Input = nil
	require.EqualValues(t, convertedLegacyTx, ethLegacyTx)

	// Verify EthTx fields
	ethTxVal, err := ethLegacyTx.ToEthTx(smsg)
	require.NoError(t, err)
	expectedHash, err := ethLegacyTx.TxHash()
	require.NoError(t, err)
	require.EqualValues(t, ethTxVal.Hash, expectedHash)
	require.Nil(t, ethTxVal.MaxFeePerGas)
	require.Nil(t, ethTxVal.MaxPriorityFeePerGas)
	require.EqualValues(t, ethTxVal.Gas, ethLegacyTx.GasLimit)
	require.EqualValues(t, ethTxVal.Value, ethLegacyTx.Value)
	require.EqualValues(t, ethTxVal.Nonce, ethLegacyTx.Nonce)
	require.EqualValues(t, ethTxVal.To, ethLegacyTx.To)
	require.EqualValues(t, ethTxVal.From, expectedFromAddr)
}

func TestFailurePaths(t *testing.T) {
	// Test case for invalid RLP
	invalidRLP := "0x08718301efc58506fc23ac008305161594104994f45d9d697ca104e5704a7b77d7fec3537c890821878651a4d70000801ba051222d91a379452395d0abaff981af4cfcc242f25cfaf947dea8245a477731f9a03a997c910b4701cca5d933fb26064ee5af7fe3236ff0ef2b58aa50b25aff8ca5"
	decoded, err := hex.DecodeString(strings.TrimPrefix(invalidRLP, "0x"))
	require.NoError(t, err)

	_, err = parseLegacyTx(decoded)
	require.Error(t, err, "Expected error for invalid RLP")

	// Test case for mangled signature
	mangledSignatureRLP := "0xf8718301efc58506fc23ac008305161594104994f45d9d697ca104e5704a7b77d7fec3537c890821878651a4d70000801ba051222d91a379452395d0abaff981af4cfcc242f25cfaf947dea8245a477731f9a03a997c910b4701cca5d933fb26064ee5af7fe3236ff0ef2b58aa50b25aff8ca5"
	decodedSig, err := hex.DecodeString(strings.TrimPrefix(mangledSignatureRLP, "0x"))
	require.NoError(t, err)

	tx, err := parseLegacyTx(decodedSig)
	require.NoError(t, err)

	ethLegacyTx, ok := tx.(*EthLegacyHomesteadTxArgs)
	require.True(t, ok)

	// Mangle R value
	ethLegacyTx.R = big.Add(ethLegacyTx.R, big.NewInt(1))

	expectedFromAddr, err := ParseEthAddress("0x32Be343B94f860124dC4fEe278FDCBD38C102D88")
	require.NoError(t, err)
	expectedFromFilecoinAddr, err := expectedFromAddr.ToFilecoinAddress()
	require.NoError(t, err)

	senderAddr, err := ethLegacyTx.Sender()
	require.NoError(t, err)
	require.NotEqual(t, senderAddr, expectedFromFilecoinAddr, "Expected sender address to not match after mangling R value")

	// Mangle V value
	ethLegacyTx.V = big.NewInt(1)
	_, err = ethLegacyTx.Sender()
	require.Error(t, err, "Expected error when V value is not 27 or 28")
}
