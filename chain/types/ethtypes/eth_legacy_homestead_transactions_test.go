package ethtypes

import (
	"encoding/hex"
	"testing"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/lib/sigs"

	"github.com/stretchr/testify/require"
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
		txArgs, err := parseLegacyHomesteadTx(mustDecodeHex(tc.RawTx))
		require.NoError(t, err)

		msgRecovered, err := txArgs.ToRlpUnsignedMsg()
		require.NoError(t, err)

		// verify signatures
		from, err := txArgs.Sender()
		require.NoError(t, err)

		smsg, err := txArgs.ToSignedMessage()
		require.NoError(t, err)

		sig := smsg.Signature.Data[:]
		sig = sig[1:]
		vValue := big.NewInt(0).SetBytes(sig[64:])
		vValue_ := big.Sub(big.NewFromGo(vValue), big.NewInt(27))
		sig[64] = byte(vValue_.Uint64())
		smsg.Signature.Data = sig

		err = sigs.Verify(&smsg.Signature, from, msgRecovered)
		require.NoError(t, err)

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
		RawTx     string
		ExpectedR string
		ExpectedS string
		ExpectedV string
		ExpectErr bool
	}{
		{
			"0xf882800182540894095e7baea6a6c7c4c2dfeb977efac326af552d8780a3deadbeef0000000101010010101010101010101010101aaabbbbbbcccccccddddddddd1ba048b55bfa915ac795c431978d8a6a992b628d557da5ff759b307d495a36649353a01fffd310ac743f371de3b9f7f9cb56c0b28ad43601b4ab949f53faa07bd2c804",
			`"0x48b55bfa915ac795c431978d8a6a992b628d557da5ff759b307d495a36649353"`,
			`"0x1fffd310ac743f371de3b9f7f9cb56c0b28ad43601b4ab949f53faa07bd2c804"`,
			`"0x1b"`,
			false,
		},
		{
			"0xf85f030182520794b94f5374fce5edbc8e2a8697c15331677e6ebf0b0a801ba098ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4aa07778cde41a8a37f6a087622b38bc201bd3e7df06dce067569d4def1b53dba98c",
			`"0x98ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4a"`,
			`"0x7778cde41a8a37f6a087622b38bc201bd3e7df06dce067569d4def1b53dba98c"`,
			`"0x1b"`,
			false,
		},
		{
			"0x00",
			`""`,
			`""`,
			`""`,
			true,
		},
	}

	for i, tc := range testcases {
		tx, err := parseLegacyHomesteadTx(mustDecodeHex(tc.RawTx))
		if tc.ExpectErr {
			require.Error(t, err)
			continue
		}
		require.Nil(t, err)

		sig, err := tx.Signature()
		require.Nil(t, err)

		tx.SetEthSignatureValues(*sig)

		marshaledR, err := tx.R.MarshalJSON()
		require.Nil(t, err)

		marshaledS, err := tx.S.MarshalJSON()
		require.Nil(t, err)

		marshaledV, err := tx.V.MarshalJSON()
		require.Nil(t, err)

		require.Equal(t, tc.ExpectedR, string(marshaledR), i)
		require.Equal(t, tc.ExpectedS, string(marshaledS), i)
		require.Equal(t, tc.ExpectedV, string(marshaledV), i)
	}
}
