package ethtypes

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
)

// encodeAuthWrapper encodes a wrapper [ list ] with one 6-tuple for convenience.
func encodeAuthWrapper(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	// wrapper [ list ]
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
	// tuple [ chain_id, address(20), nonce, y_parity, r, s ]
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 314))
	// 20-byte address
	var addr [20]byte
	for i := range addr {
		addr[i] = 0xaa
	}
	require.NoError(t, cbg.WriteByteArray(&buf, addr[:]))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0)) // nonce
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0)) // y_parity
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))              // r
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))              // s
	return buf.Bytes()
}

func TestEthTransactionFromSignedMessage_7702_Decodes(t *testing.T) {
	// Setup: set EthAccountApplyAndCallActorAddr to ID:18
	id18, _ := address.NewIDAddress(18)
	EthAccountApplyAndCallActorAddr = id18
	// From must be an eth (f4) address
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x11
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)

	// Build SignedMessage targeting EthAccount.ApplyAndCall
	msg := types.Message{
		Version:    0,
		To:         EthAccountApplyAndCallActorAddr,
		From:       from,
		Nonce:      0,
		Value:      types.NewInt(0),
		Method:     abi.MethodNum(MethodHash("ApplyAndCall")),
		GasLimit:   100000,
		GasFeeCap:  types.NewInt(1),
		GasPremium: types.NewInt(1),
		Params:     encodeAuthWrapper(t),
	}
	// Fake a delegated 65-byte signature r||s||v where r,s=1 and v=0
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}

	tx, err := EthTransactionFromSignedFilecoinMessage(smsg)
	require.NoError(t, err)
	// Expect a 0x04 typed tx and authorizationList echoed
	require.EqualValues(t, EIP7702TxType, tx.Type())
	eth, err := tx.ToEthTx(smsg)
	require.NoError(t, err)
	require.Len(t, eth.AuthorizationList, 1)
	require.EqualValues(t, 314, eth.AuthorizationList[0].ChainID)
}

func TestEthTransactionFromSignedMessage_7702_MultiTupleDecodes(t *testing.T) {
	// Setup ID:18 EthAccount.ApplyAndCall address and f4 sender
	id18, _ := address.NewIDAddress(18)
	EthAccountApplyAndCallActorAddr = id18
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x22
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)

	// Build params wrapper with two tuples
	var buf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	encTup := func(chain uint64, nonce uint64) {
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, chain))
		var a [20]byte
		for i := range a {
			a[i] = 0xAA
		}
		require.NoError(t, cbg.WriteByteArray(&buf, a[:]))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, nonce))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
		require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
		require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
	}
	encTup(314, 0)
	encTup(314, 1)

	msg := types.Message{To: EthAccountApplyAndCallActorAddr, From: from, Method: abi.MethodNum(MethodHash("ApplyAndCall")), Params: buf.Bytes(), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1), Value: types.NewInt(0)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}

	tx, err := EthTransactionFromSignedFilecoinMessage(smsg)
	require.NoError(t, err)
	eth, err := tx.ToEthTx(smsg)
	require.NoError(t, err)
	require.Len(t, eth.AuthorizationList, 2)
}

func TestEthTransactionFromSignedMessage_NonDelegatedSigRejected(t *testing.T) {
	// Setup EthAccount.ApplyAndCall address; signature type is wrong (secp256k1)
	id18, _ := address.NewIDAddress(18)
	EthAccountApplyAndCallActorAddr = id18
	// Sender can be anything; rejection occurs earlier on sig type
	from, _ := address.NewIDAddress(1001)
	msg := types.Message{To: EthAccountApplyAndCallActorAddr, From: from, Method: abi.MethodNum(MethodHash("ApplyAndCall"))}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeSecp256k1, Data: make([]byte, 65)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}
	_, err := EthTransactionFromSignedFilecoinMessage(smsg)
	require.Error(t, err)
}

func TestEthTransactionFromSignedMessage_SenderNotEthRejected(t *testing.T) {
	// Delegated signature but non-f4 sender should be rejected
	id18, _ := address.NewIDAddress(18)
	EthAccountApplyAndCallActorAddr = id18
	// Non-eth sender: ID address
	from, _ := address.NewIDAddress(42)
	msg := types.Message{To: EthAccountApplyAndCallActorAddr, From: from, Method: abi.MethodNum(MethodHash("ApplyAndCall"))}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: make([]byte, 65)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}
	_, err := EthTransactionFromSignedFilecoinMessage(smsg)
	require.Error(t, err)
}

func TestEthTransactionFromSignedMessage_7702_BadCBORRejected(t *testing.T) {
	// Setup ID:18 EthAccount.ApplyAndCall address and f4 sender
	id18, _ := address.NewIDAddress(18)
	EthAccountApplyAndCallActorAddr = id18
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x33
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)

	// Malformed CBOR params (unsigned int header instead of array)
	var buf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 7))

	msg := types.Message{To: EthAccountApplyAndCallActorAddr, From: from, Method: abi.MethodNum(MethodHash("ApplyAndCall")), Params: buf.Bytes(), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}

	_, err = EthTransactionFromSignedFilecoinMessage(smsg)
	require.Error(t, err)
}

func TestEthTransactionFromSignedMessage_7702_WrongTupleArityRejected(t *testing.T) {
	id18, _ := address.NewIDAddress(18)
	EthAccountApplyAndCallActorAddr = id18
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x44
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
	// Wrong arity: tuple with 5 elements instead of 6
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 5))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 314))
	var addr [20]byte
	for i := range addr {
		addr[i] = 0xaa
	}
	require.NoError(t, cbg.WriteByteArray(&buf, addr[:]))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))

	msg := types.Message{To: EthAccountApplyAndCallActorAddr, From: from, Method: abi.MethodNum(MethodHash("ApplyAndCall")), Params: buf.Bytes(), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}
	_, err = EthTransactionFromSignedFilecoinMessage(smsg)
	require.Error(t, err)
}

func TestEthTransactionFromSignedMessage_7702_BadAddressLengthRejected(t *testing.T) {
	id18, _ := address.NewIDAddress(18)
	EthAccountApplyAndCallActorAddr = id18
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x55
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
	// Tuple with wrong address byte string length (19 instead of 20)
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 314))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajByteString, 19))
	_, _ = buf.Write(make([]byte, 19))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))

	msg := types.Message{To: EthAccountApplyAndCallActorAddr, From: from, Method: abi.MethodNum(MethodHash("ApplyAndCall")), Params: buf.Bytes(), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}
	_, err = EthTransactionFromSignedFilecoinMessage(smsg)
	require.Error(t, err)
}

func TestEthTransactionFromSignedMessage_7702_EmptyAuthListRejected(t *testing.T) {
	id18, _ := address.NewIDAddress(18)
	EthAccountApplyAndCallActorAddr = id18
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x66
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)

	var buf bytes.Buffer
	// Top-level [ list, call ] but with empty auth list (list length 0)
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 0))
	// Minimal call tuple [to(20), value, input]
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
	require.NoError(t, cbg.WriteByteArray(&buf, make([]byte, 20)))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{}))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{}))

	msg := types.Message{To: EthAccountApplyAndCallActorAddr, From: from, Method: abi.MethodNum(MethodHash("ApplyAndCall")), Params: buf.Bytes(), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}
	_, err = EthTransactionFromSignedFilecoinMessage(smsg)
	require.Error(t, err)
}

func TestEthTransactionFromSignedMessage_7702_BadYParityTypeRejected(t *testing.T) {
	id18, _ := address.NewIDAddress(18)
	EthAccountApplyAndCallActorAddr = id18
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x77
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
	// Tuple with wrong y_parity major (byte string instead of unsigned int)
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 314))
	var addr [20]byte
	for i := range addr {
		addr[i] = 0xaa
	}
	require.NoError(t, cbg.WriteByteArray(&buf, addr[:]))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0)) // nonce ok
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajByteString, 1))  // y_parity wrong major
	_, _ = buf.Write([]byte{0x00})
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))

	msg := types.Message{To: EthAccountApplyAndCallActorAddr, From: from, Method: abi.MethodNum(MethodHash("ApplyAndCall")), Params: buf.Bytes(), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}
	_, err = EthTransactionFromSignedFilecoinMessage(smsg)
	require.Error(t, err)
}
