package ethtypes

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build/buildconstants"
	ltypes "github.com/filecoin-project/lotus/chain/types"
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

func TestEIP7702_ToEthTx_CarriesAuthorizationList(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	tx := &Eth7702TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Nonce:                42,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		AuthorizationList: []EthAuthorization{
			{ChainID: EthUint64(buildconstants.Eip155ChainId), Address: to, Nonce: EthUint64(7), YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(2))},
		},
		V: big.NewInt(0), R: big.NewInt(1), S: big.NewInt(1),
	}
	// Fake signed message to pass From address
	fromFC, err := (EthAddress{}).ToFilecoinAddress()
	require.NoError(t, err)
	sm := &ltypes.SignedMessage{Message: ltypes.Message{From: fromFC}}
	ethTx, err := tx.ToEthTx(sm)
	require.NoError(t, err)
	require.Equal(t, 1, len(ethTx.AuthorizationList))
}

func TestEIP7702_ToRlpUnsigned_HasTypePrefix(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	tx := &Eth7702TxArgs{
		ChainID:              1,
		Nonce:                0,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		AuthorizationList: []EthAuthorization{
			{ChainID: 1, Address: to, Nonce: 0, YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))},
		},
	}
	enc, err := tx.ToRlpUnsignedMsg()
	require.NoError(t, err)
	require.Greater(t, len(enc), 1)
	require.Equal(t, byte(EIP7702TxType), enc[0])
}

func TestEIP7702_TxHash_VariesByAuthList(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	base := Eth7702TxArgs{
		ChainID:              1,
		Nonce:                0,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		V:                    big.NewInt(0), R: big.NewInt(1), S: big.NewInt(1),
	}
	tx1 := base
	tx1.AuthorizationList = []EthAuthorization{{ChainID: 1, Address: to, Nonce: 0, YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))}}
	h1, err := tx1.TxHash()
	require.NoError(t, err)

	tx2 := base
	tx2.AuthorizationList = []EthAuthorization{{ChainID: 1, Address: to, Nonce: 1, YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))}}
	h2, err := tx2.TxHash()
	require.NoError(t, err)
	require.NotEqual(t, h1, h2)
}

func TestEIP7702_ToUnsignedFilecoinMessage_InvalidChain(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	tx := &Eth7702TxArgs{
		ChainID:              9999, // invalid
		Nonce:                0,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		AuthorizationList: []EthAuthorization{
			{ChainID: 1, Address: to, Nonce: 0, YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))},
		},
		V: big.NewInt(0), R: big.NewInt(1), S: big.NewInt(1),
	}
	_, err := tx.ToUnsignedFilecoinMessage(address.Undef)
	require.Error(t, err)
}

func TestEIP7702_RLPMultiAuth_RoundTrip(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	var a1, a2 EthAddress
	copy(a1[:], mustHex(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	copy(a2[:], mustHex(t, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))

	tx := &Eth7702TxArgs{
		ChainID:              1,
		Nonce:                1,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		AuthorizationList: []EthAuthorization{
			{ChainID: 1, Address: a1, Nonce: 0, YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(2))},
			{ChainID: 1, Address: a2, Nonce: 1, YParity: 1, R: EthBigInt(big.NewInt(3)), S: EthBigInt(big.NewInt(4))},
		},
		V: big.NewInt(0), R: big.NewInt(1), S: big.NewInt(1),
	}
	enc, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	dec, err := parseEip7702Tx(enc)
	require.NoError(t, err)
	require.Len(t, dec.AuthorizationList, 2)
	require.Equal(t, a1, dec.AuthorizationList[0].Address)
	require.Equal(t, a2, dec.AuthorizationList[1].Address)
}

func TestEIP7702_EmptyAuthorizationListRejected(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))

	tx := &Eth7702TxArgs{
		ChainID:              1,
		Nonce:                5,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		Input:                nil,
		AuthorizationList:    nil, // empty
		V:                    big.NewInt(0),
		R:                    big.NewInt(1),
		S:                    big.NewInt(1),
	}

	enc, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)

	_, err = parseEip7702Tx(enc)
	require.Error(t, err)
}

func TestEIP7702_NonEmptyAccessListRejected(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	var authAddr EthAddress
	copy(authAddr[:], mustHex(t, "0x2222222222222222222222222222222222222222"))

	// Build fields manually to inject non-empty access list at index 8
	chainId, _ := formatInt(1)
	nonce, _ := formatInt(5)
	maxPrio, _ := formatBigInt(big.NewInt(100))
	maxFee, _ := formatBigInt(big.NewInt(200))
	gasLimit, _ := formatInt(21000)
	value, _ := formatBigInt(big.NewInt(0))
	input := []byte{0xde, 0xad}

	// Authorization tuple
	ai, _ := formatInt(1)
	ni, _ := formatInt(7)
	yp, _ := formatInt(0)
	ri, _ := formatBigInt(big.NewInt(1))
	si, _ := formatBigInt(big.NewInt(2))
	authTuple := []interface{}{ai, authAddr[:], ni, yp, ri, si}
	authList := []interface{}{authTuple}

	// Non-empty access list (one dummy element)
	accessList := []interface{}{[]byte{0x01}}

	base := []interface{}{
		chainId,
		nonce,
		maxPrio,
		maxFee,
		gasLimit,
		formatEthAddr(&to),
		value,
		input,
		accessList, // should trigger error
		authList,
	}

	// Append signature fields
	sig, _ := packSigFields(big.NewInt(1), big.NewInt(3), big.NewInt(4))
	full := append(base, sig...)

	payload, err := EncodeRLP(full)
	require.NoError(t, err)
	enc := append([]byte{EIP7702TxType}, payload...)

	_, err = parseEip7702Tx(enc)
	require.Error(t, err)
}

func TestEIP7702_BadAuthorizationAddressLengthRejected(t *testing.T) {
	// Build 0x04 tx with an authorization tuple whose address is 19 bytes
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	chainId, _ := formatInt(1)
	nonce, _ := formatInt(1)
	maxPrio, _ := formatBigInt(big.NewInt(1))
	maxFee, _ := formatBigInt(big.NewInt(1))
	gasLimit, _ := formatInt(21000)
	value, _ := formatBigInt(big.NewInt(0))
	input := []byte{}
	ai, _ := formatInt(1)
	ni, _ := formatInt(0)
	yp, _ := formatInt(0)
	ri, _ := formatBigInt(big.NewInt(1))
	si, _ := formatBigInt(big.NewInt(1))
	badAddr := make([]byte, 19)
	// Tuple with 19-byte address
	authTuple := []interface{}{ai, badAddr, ni, yp, ri, si}
	base := []interface{}{
		chainId, nonce, maxPrio, maxFee, gasLimit, formatEthAddr(&to), value, input,
		[]interface{}{}, // access list
		[]interface{}{authTuple},
	}
	sig, _ := packSigFields(big.NewInt(0), big.NewInt(1), big.NewInt(1))
	payload, _ := EncodeRLP(append(base, sig...))
	enc := append([]byte{EIP7702TxType}, payload...)
	_, err := parseEip7702Tx(enc)
	require.Error(t, err)
}

func TestEIP7702_BadOuterLenRejected(t *testing.T) {
	// Construct an RLP list with only 12 elements (missing one), should fail
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	chainId, _ := formatInt(1)
	nonce, _ := formatInt(1)
	maxPrio, _ := formatBigInt(big.NewInt(1))
	maxFee, _ := formatBigInt(big.NewInt(1))
	gasLimit, _ := formatInt(21000)
	value, _ := formatBigInt(big.NewInt(0))
	input := []byte{}
	// Authorization list with one valid-looking tuple
	var authAddr EthAddress
	copy(authAddr[:], mustHex(t, "0x2222222222222222222222222222222222222222"))
	ai, _ := formatInt(1)
	ni, _ := formatInt(0)
	yp, _ := formatInt(0)
	ri, _ := formatBigInt(big.NewInt(1))
	si, _ := formatBigInt(big.NewInt(1))
	authList := []interface{}{[]interface{}{ai, authAddr[:], ni, yp, ri, si}}
	// Build only 12 elements (omit s for outer signature later so list size is wrong)
	base := []interface{}{chainId, nonce, maxPrio, maxFee, gasLimit, formatEthAddr(&to), value, input, []interface{}{}, authList, []byte{0x00}, []byte{0x01}}
	payload, _ := EncodeRLP(base)
	enc := append([]byte{EIP7702TxType}, payload...)
	_, err := parseEip7702Tx(enc)
	require.Error(t, err)
}

func TestEIP7702_AuthorizationListMustBeList(t *testing.T) {
	// Insert a non-list at authorizationList position
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	chainId, _ := formatInt(1)
	nonce, _ := formatInt(1)
	maxPrio, _ := formatBigInt(big.NewInt(1))
	maxFee, _ := formatBigInt(big.NewInt(1))
	gasLimit, _ := formatInt(21000)
	value, _ := formatBigInt(big.NewInt(0))
	input := []byte{}
	base := []interface{}{
		chainId, nonce, maxPrio, maxFee, gasLimit, formatEthAddr(&to), value, input,
		[]interface{}{},          // access list (empty)
		[]byte{0x01, 0x02, 0x03}, // not a list
	}
	sig, _ := packSigFields(big.NewInt(0), big.NewInt(1), big.NewInt(1))
	payload, _ := EncodeRLP(append(base, sig...))
	enc := append([]byte{EIP7702TxType}, payload...)
	_, err := parseEip7702Tx(enc)
	require.Error(t, err)
}

func TestEIP7702_VParityRejected(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))

	tx := &Eth7702TxArgs{
		ChainID:              1,
		Nonce:                1,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		AuthorizationList: []EthAuthorization{
			{ChainID: EthUint64(1), Address: to, Nonce: EthUint64(1), YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))},
		},
		V: big.NewInt(2), // invalid
		R: big.NewInt(1),
		S: big.NewInt(1),
	}
	enc, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	_, err = parseEip7702Tx(enc)
	require.Error(t, err)
}

func TestEIP7702_AuthorizationYParityRejected(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	tx := &Eth7702TxArgs{
		ChainID:              1,
		Nonce:                1,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		AuthorizationList: []EthAuthorization{
			{ChainID: EthUint64(1), Address: to, Nonce: EthUint64(1), YParity: 2, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))},
		},
		V: big.NewInt(0), R: big.NewInt(1), S: big.NewInt(1),
	}
	enc, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	_, err = parseEip7702Tx(enc)
	require.Error(t, err)
}

func TestEIP7702_ToUnsignedFilecoinMessage_Guard(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	tx := &Eth7702TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Nonce:                0,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		AuthorizationList: []EthAuthorization{
			{ChainID: EthUint64(buildconstants.Eip155ChainId), Address: to, Nonce: EthUint64(0), YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))},
		},
		V: big.NewInt(0),
		R: big.NewInt(1),
		S: big.NewInt(1),
	}
	_, err := tx.ToUnsignedFilecoinMessage(address.Undef)
	require.Error(t, err)
}

// Feature-gated test for EthAccount.ApplyAndCall receiver is in eth_7702_transactions_env_enabled_test.go

func TestEIP7702_AuthorizationTupleWrongArityRejected(t *testing.T) {
	// Build fields manually and inject an authorization tuple with only 5 items
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	var authAddr EthAddress
	copy(authAddr[:], mustHex(t, "0x2222222222222222222222222222222222222222"))

	chainId, _ := formatInt(1)
	nonce, _ := formatInt(5)
	maxPrio, _ := formatBigInt(big.NewInt(100))
	maxFee, _ := formatBigInt(big.NewInt(200))
	gasLimit, _ := formatInt(21000)
	value, _ := formatBigInt(big.NewInt(0))
	input := []byte{}

	ai, _ := formatInt(1)
	ni, _ := formatInt(7)
	yp, _ := formatInt(0)
	ri, _ := formatBigInt(big.NewInt(1))
	// si omitted on purpose
	badTuple := []interface{}{ai, authAddr[:], ni, yp, ri}
	authList := []interface{}{badTuple}

	base := []interface{}{
		chainId,
		nonce,
		maxPrio,
		maxFee,
		gasLimit,
		formatEthAddr(&to),
		value,
		input,
		[]interface{}{}, // access list
		authList,
	}
	sig, _ := packSigFields(big.NewInt(0), big.NewInt(1), big.NewInt(1))
	payload, _ := EncodeRLP(append(base, sig...))
	enc := append([]byte{EIP7702TxType}, payload...)

	_, err := parseEip7702Tx(enc)
	require.Error(t, err)
}

func TestEIP7702_InitialiseSignature_SetsVRandS(t *testing.T) {
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	tx := &Eth7702TxArgs{
		ChainID:              1,
		Nonce:                1,
		To:                   &to,
		Value:                big.NewInt(0),
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             21000,
		AuthorizationList: []EthAuthorization{
			{ChainID: 1, Address: to, Nonce: 0, YParity: 0, R: EthBigInt(big.NewInt(1)), S: EthBigInt(big.NewInt(1))},
		},
	}
	// Build a 65-byte signature r||s||v with r=2, s=3, v=1
	sig := make([]byte, 65)
	sig[31] = 2 // r
	sig[63] = 3 // s
	sig[64] = 1 // v (y_parity)
	require.NoError(t, tx.InitialiseSignature(typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: sig}))
	require.Equal(t, "2", tx.R.String())
	require.Equal(t, "3", tx.S.String())
	require.Equal(t, "1", tx.V.String())
}

func TestEIP7702_AuthorizationTuple_Uint64BoundaryDecode(t *testing.T) {
	// Build fields manually and inject authorization tuple with chainId and nonce near MaxUint64.
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))

	chainId, _ := formatInt(1)
	nonce, _ := formatInt(1)
	maxPrio, _ := formatBigInt(big.NewInt(1))
	maxFee, _ := formatBigInt(big.NewInt(1))
	gasLimit, _ := formatInt(21000)
	value, _ := formatBigInt(big.NewInt(0))
	input := []byte{}

	// Inner tuple chainId/nonce set to max uint64 values
	ai, _ := formatUint64(^uint64(0))
	ni, _ := formatUint64(^uint64(0))
	yp, _ := formatInt(0)
	ri, _ := formatBigInt(big.NewInt(1))
	si, _ := formatBigInt(big.NewInt(1))
	var authAddr EthAddress
	copy(authAddr[:], mustHex(t, "0x2222222222222222222222222222222222222222"))
	authTuple := []interface{}{ai, authAddr[:], ni, yp, ri, si}
	authList := []interface{}{authTuple}

	base := []interface{}{
		chainId,
		nonce,
		maxPrio,
		maxFee,
		gasLimit,
		formatEthAddr(&to),
		value,
		input,
		[]interface{}{}, // access list
		authList,
	}
	sig, _ := packSigFields(big.NewInt(0), big.NewInt(1), big.NewInt(1))
	payload, _ := EncodeRLP(append(base, sig...))
	enc := append([]byte{EIP7702TxType}, payload...)

	dec, err := parseEip7702Tx(enc)
	require.NoError(t, err)
	require.Len(t, dec.AuthorizationList, 1)
	require.Equal(t, EthUint64(^uint64(0)), dec.AuthorizationList[0].ChainID)
	require.Equal(t, EthUint64(^uint64(0)), dec.AuthorizationList[0].Nonce)
}

func TestEIP7702_InitialiseSignature_WrongTypeRejected(t *testing.T) {
	var to EthAddress
	tx := &Eth7702TxArgs{To: &to}
	// SECP256K1 type should be rejected for 7702
	err := tx.InitialiseSignature(typescrypto.Signature{Type: typescrypto.SigTypeSecp256k1, Data: make([]byte, 65)})
	require.Error(t, err)
}

func TestEIP7702_InitialiseSignature_WrongLenRejected(t *testing.T) {
	var to EthAddress
	tx := &Eth7702TxArgs{To: &to}
	// Delegated but wrong length (<65)
	err := tx.InitialiseSignature(typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: make([]byte, 64)})
	require.Error(t, err)
}
