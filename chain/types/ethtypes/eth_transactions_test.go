package ethtypes

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestEthTransactionFromSignedFilecoinMessage(t *testing.T) {
	eip1559sig := make([]byte, 65)
	eip1559sig[0] = 1

	legacySig := make([]byte, 66)
	legacySig[0] = 1
	legacySig[65] = 27

	pubKeyHex := "0x04cfecc0520d906cbfea387759246e89d85e2998843e56ad1c41de247ce10b3e4c453aa73c8de13c178d94461b6fa3f8b6f74406ce43d2fbab6992d0b283394242"
	pubk := mustDecodeHex(pubKeyHex)
	addrHash, err := EthAddressFromPubKey(pubk)
	require.NoError(t, err)
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, addrHash)
	require.NoError(t, err)

	fromEth, err := EthAddressFromFilecoinAddress(from)
	require.NoError(t, err)

	to, err := address.NewIDAddress(1)
	require.NoError(t, err)

	toEth, err := EthAddressFromFilecoinAddress(to)
	require.NoError(t, err)

	tcs := map[string]struct {
		msg          *types.SignedMessage
		expectedErr  string
		validateFunc func(t *testing.T, smsg *types.SignedMessage, tx EthTransaction)
	}{
		"empty": {
			expectedErr: "signed message is nil",
		},
		"invalid-signature": {
			msg: &types.SignedMessage{
				Message: types.Message{
					To:     builtintypes.EthereumAddressManagerActorAddr,
					From:   from,
					Method: builtintypes.MethodsEAM.CreateExternal,
				},
				Signature: crypto.Signature{
					Type: crypto.SigTypeDelegated,
					Data: []byte{1},
				},
			},
			expectedErr: "unsupported signature length",
		},
		"valid-eip1559": {
			msg: &types.SignedMessage{
				Message: types.Message{
					From:       from,
					To:         to,
					Value:      big.NewInt(10),
					GasFeeCap:  big.NewInt(11),
					GasPremium: big.NewInt(12),
					GasLimit:   13,
					Nonce:      14,
					Method:     builtintypes.MethodsEVM.InvokeContract,
				},
				Signature: crypto.Signature{
					Type: crypto.SigTypeDelegated,
					Data: eip1559sig,
				},
			},
			validateFunc: func(t *testing.T, smsg *types.SignedMessage, tx EthTransaction) {
				eip1559tx := tx.(*Eth1559TxArgs)
				require.Equal(t, big.NewInt(10), eip1559tx.Value)
				require.Equal(t, big.NewInt(11), eip1559tx.MaxFeePerGas)
				require.Equal(t, big.NewInt(12), eip1559tx.MaxPriorityFeePerGas)
				require.EqualValues(t, uint64(13), eip1559tx.GasLimit)
				require.EqualValues(t, uint64(14), eip1559tx.Nonce)
				require.EqualValues(t, toEth, *eip1559tx.To)
				require.EqualValues(t, 314, eip1559tx.ChainID)
				require.Empty(t, eip1559tx.Input)

				ethTx, err := tx.ToEthTx(smsg)
				require.NoError(t, err)
				require.EqualValues(t, 314, ethTx.ChainID)
				require.EqualValues(t, 14, ethTx.Nonce)
				hash, err := eip1559tx.TxHash()
				require.NoError(t, err)
				require.EqualValues(t, hash, ethTx.Hash)
				require.EqualValues(t, fromEth, ethTx.From)
				require.EqualValues(t, toEth, *ethTx.To)
				require.EqualValues(t, big.NewInt(10), ethTx.Value)
				require.EqualValues(t, 13, ethTx.Gas)
				require.EqualValues(t, big.NewInt(11), *ethTx.MaxFeePerGas)
				require.EqualValues(t, big.NewInt(12), *ethTx.MaxPriorityFeePerGas)
				require.Nil(t, ethTx.GasPrice)
				require.Empty(t, ethTx.AccessList)
			},
		},
		"valid-legacy": {
			msg: &types.SignedMessage{
				Message: types.Message{
					From:       from,
					To:         to,
					Value:      big.NewInt(10),
					GasFeeCap:  big.NewInt(11),
					GasPremium: big.NewInt(12),
					GasLimit:   13,
					Nonce:      14,
					Method:     builtintypes.MethodsEVM.InvokeContract,
				},
				Signature: crypto.Signature{
					Type: crypto.SigTypeDelegated,
					Data: legacySig,
				},
			},
			validateFunc: func(t *testing.T, smsg *types.SignedMessage, tx EthTransaction) {
				legacyTx := tx.(*EthLegacyHomesteadTxArgs)
				require.Equal(t, big.NewInt(10), legacyTx.Value)
				require.EqualValues(t, uint64(13), legacyTx.GasLimit)
				require.EqualValues(t, uint64(14), legacyTx.Nonce)
				require.EqualValues(t, toEth, *legacyTx.To)
				require.EqualValues(t, big.NewInt(11), legacyTx.GasPrice)
				require.Empty(t, legacyTx.Input)

				ethTx, err := tx.ToEthTx(smsg)
				require.NoError(t, err)
				require.EqualValues(t, 0, ethTx.ChainID)
				require.EqualValues(t, 14, ethTx.Nonce)
				hash, err := legacyTx.TxHash()
				require.NoError(t, err)
				require.EqualValues(t, big.NewInt(11), *ethTx.GasPrice)
				require.EqualValues(t, hash, ethTx.Hash)
				require.EqualValues(t, fromEth, ethTx.From)
				require.EqualValues(t, toEth, *ethTx.To)
				require.EqualValues(t, big.NewInt(10), ethTx.Value)
				require.EqualValues(t, 13, ethTx.Gas)
				require.Nil(t, ethTx.MaxFeePerGas)
				require.Nil(t, ethTx.MaxPriorityFeePerGas)
				require.Empty(t, ethTx.AccessList)
				require.EqualValues(t, big.NewInt(27), ethTx.V)
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			tx, err := EthTransactionFromSignedFilecoinMessage(tc.msg)
			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
			if tc.validateFunc != nil {
				tc.validateFunc(t, tc.msg, tx)
			}
		})
	}
}

func TestParseEthTransactionNonMinimalInts(t *testing.T) {
	// RLP integers carry no leading zero bytes, so a field encoded with one
	// decodes to a value that already has a shorter encoding. Both forms parse
	// to the same transaction and recover the same sender, which would let
	// anyone rewrite the bytes of someone else's signed transaction and get a
	// different EthHashFromTxBytes out of it.
	testcases := []struct {
		name  string
		rawTx string
	}{
		{
			// nonce 0x02 re-encoded as the two byte string {0x00, 0x02}
			"eip1559 nonce",
			"0x02f85b8401df5e768200028301d69083086a5e835532dd808080c080a0457e33227ac7ceee2ef121755e26b872b6fb04221993f9939349bb7b0a3e1595a02d8ef379e1d2a9e30fa61c92623cc9ed72d80cf6a48cfea341cb916bcc0a81bc",
		},
		{
			// maxPriorityFeePerGas 0x01d690 re-encoded as {0x00, 0x01, 0xd6, 0x90}
			"eip1559 maxPriorityFeePerGas",
			"0x02f85a8401df5e7602840001d69083086a5e835532dd808080c080a0457e33227ac7ceee2ef121755e26b872b6fb04221993f9939349bb7b0a3e1595a02d8ef379e1d2a9e30fa61c92623cc9ed72d80cf6a48cfea341cb916bcc0a81bc",
		},
		{
			// nonce 0x03 re-encoded as the two byte string {0x00, 0x03}
			"legacy nonce",
			"0xf8618200030182520794b94f5374fce5edbc8e2a8697c15331677e6ebf0b0a801ba098ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4aa07778cde41a8a37f6a087622b38bc201bd3e7df06dce067569d4def1b53dba98c",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseEthTransaction(mustDecodeHex(tc.rawTx))
			require.ErrorContains(t, err, "non-minimal encoding")
		})
	}

	// the minimally encoded forms of the same transactions still parse
	for _, rawTx := range []string{
		"0x02f8598401df5e76028301d69083086a5e835532dd808080c080a0457e33227ac7ceee2ef121755e26b872b6fb04221993f9939349bb7b0a3e1595a02d8ef379e1d2a9e30fa61c92623cc9ed72d80cf6a48cfea341cb916bcc0a81bc",
		"0xf85f030182520794b94f5374fce5edbc8e2a8697c15331677e6ebf0b0a801ba098ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4aa07778cde41a8a37f6a087622b38bc201bd3e7df06dce067569d4def1b53dba98c",
	} {
		_, err := ParseEthTransaction(mustDecodeHex(rawTx))
		require.NoError(t, err)
	}
}
