package ethtypes

import (
	"bytes"
	"fmt"

	"github.com/filecoin-project/go-address"
	gocrypto "github.com/filecoin-project/go-crypto"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/types"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/crypto/sha3"
	"golang.org/x/xerrors"
)

// define a one byte prefix for legacy eth transactions
const LegacyEthTxPrefix = 0x80

type LegacyEthTx struct {
	Nonce    int         `json:"nonce"`
	GasPrice big.Int     `json:"gasPrice"`
	GasLimit int         `json:"gasLimit"`
	To       *EthAddress `json:"to"`
	Value    big.Int     `json:"value"`
	Input    []byte      `json:"input"`
	V        big.Int     `json:"v"`
	R        big.Int     `json:"r"`
	S        big.Int     `json:"s"`
}

func parseLegacyTx(data []byte) (*LegacyEthTx, error) {
	if data[0] <= 0x7f {
		return nil, fmt.Errorf("not a legacy eth transaction")
	}

	d, err := DecodeRLP(data)
	if err != nil {
		return nil, err
	}
	decoded, ok := d.([]interface{})
	if !ok {
		return nil, fmt.Errorf("not a Legacy transaction: decoded data is not a list")
	}

	if len(decoded) != 9 {
		return nil, fmt.Errorf("not a Legacy transaction: should have 9 elements in the rlp list")
	}

	nonce, err := parseInt(decoded[0])
	if err != nil {
		return nil, err
	}

	gasPrice, err := parseBigInt(decoded[1])
	if err != nil {
		return nil, err
	}

	gasLimit, err := parseInt(decoded[2])
	if err != nil {
		return nil, err
	}

	to, err := parseEthAddr(decoded[3])
	if err != nil {
		return nil, err
	}

	value, err := parseBigInt(decoded[4])
	if err != nil {
		return nil, err
	}

	input, ok := decoded[5].([]byte)
	if !ok {
		return nil, fmt.Errorf("input is not a byte slice")
	}

	v, err := parseBigInt(decoded[6])
	if err != nil {
		return nil, err
	}

	r, err := parseBigInt(decoded[7])
	if err != nil {
		return nil, err
	}

	s, err := parseBigInt(decoded[8])
	if err != nil {
		return nil, err
	}

	return &LegacyEthTx{
		Nonce:    nonce,
		GasPrice: gasPrice,
		GasLimit: gasLimit,
		To:       to,
		Value:    value,
		Input:    input,
		V:        v,
		R:        r,
		S:        s,
	}, nil
}

func (tx *LegacyEthTx) ToUnsignedMessage(from address.Address) (*types.Message, error) {
	var err error
	var params []byte
	if len(tx.Input) > 0 {
		buf := new(bytes.Buffer)
		if err = cbg.WriteByteArray(buf, tx.Input); err != nil {
			return nil, xerrors.Errorf("failed to write input args: %w", err)
		}
		params = buf.Bytes()
	}

	var to address.Address
	var method abi.MethodNum
	// nil indicates the EAM, only CreateExternal is allowed
	if tx.To == nil {
		method = builtintypes.MethodsEAM.CreateExternal
		to = builtintypes.EthereumAddressManagerActorAddr
	} else {
		method = builtintypes.MethodsEVM.InvokeContract
		to, err = tx.To.ToFilecoinAddress()
		if err != nil {
			return nil, xerrors.Errorf("failed to convert To into filecoin addr: %w", err)
		}
	}

	return &types.Message{
		Version:    0,
		To:         to,
		From:       from,
		Nonce:      uint64(tx.Nonce),
		Value:      tx.Value,
		GasLimit:   int64(tx.GasLimit),
		GasFeeCap:  tx.GasPrice,
		GasPremium: tx.GasPrice,
		Method:     method,
		Params:     params,
	}, nil
}

func (tx *LegacyEthTx) ToSignedMessage() (*types.SignedMessage, error) {
	from, err := tx.Sender()
	if err != nil {
		return nil, xerrors.Errorf("failed to calculate sender: %w", err)
	}

	unsignedMsg, err := tx.ToUnsignedMessage(from)
	if err != nil {
		return nil, xerrors.Errorf("failed to convert to unsigned msg: %w", err)
	}

	siggy, err := tx.Signature()
	if err != nil {
		return nil, xerrors.Errorf("failed to calculate signature: %w", err)
	}

	return &types.SignedMessage{
		Message:   *unsignedMsg,
		Signature: *siggy,
	}, nil
}

func (tx *LegacyEthTx) packTxFields() ([]interface{}, error) {
	nonce, err := formatInt(tx.Nonce)
	if err != nil {
		return nil, err
	}

	// format gas price
	gasPrice, err := formatBigInt(tx.GasPrice)
	if err != nil {
		return nil, err
	}

	gasLimit, err := formatInt(tx.GasLimit)
	if err != nil {
		return nil, err
	}

	value, err := formatBigInt(tx.Value)
	if err != nil {
		return nil, err
	}

	res := []interface{}{
		nonce,
		gasPrice,
		gasLimit,
		formatEthAddr(tx.To),
		value,
		tx.Input,
	}
	return res, nil
}

func (tx *LegacyEthTx) ToRlpUnsignedMsg() ([]byte, error) {
	packedFields, err := tx.packTxFields()
	if err != nil {
		return nil, err
	}
	encoded, err := EncodeRLP(packedFields)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func (tx *LegacyEthTx) Signature() (*typescrypto.Signature, error) {
	r := tx.R.Int.Bytes()
	s := tx.S.Int.Bytes()
	v := tx.V.Int.Bytes()

	sig := append([]byte{}, padLeadingZeros(r, 32)...)
	sig = append(sig, padLeadingZeros(s, 32)...)
	if len(v) == 0 {
		sig = append(sig, 0)
	} else {
		sig = append(sig, v[0])
	}
	if len(sig) != 65 {
		return nil, fmt.Errorf("signature is not 65 bytes")
	}

	// pre-pend a one byte marker so nodes know that this is a legacy transaction
	sig = append([]byte{LegacyEthTxPrefix}, sig...)

	return &typescrypto.Signature{
		Type: typescrypto.SigTypeDelegated, Data: sig,
	}, nil
}

func (tx *LegacyEthTx) Sender() (address.Address, error) {
	msg, err := tx.ToRlpUnsignedMsg()
	if err != nil {
		return address.Undef, err
	}

	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(msg)
	hash := hasher.Sum(nil)

	sig, err := tx.Signature()
	if err != nil {
		return address.Undef, err
	}

	// remove the first byte before extracting public key as the first byte is the prefix for legacy txs
	pubk, err := gocrypto.EcRecover(hash, sig.Data[1:])
	if err != nil {
		return address.Undef, err
	}

	ethAddr, err := EthAddressFromPubKey(pubk)
	if err != nil {
		return address.Undef, err
	}

	ea, err := CastEthAddress(ethAddr)
	if err != nil {
		return address.Undef, err
	}

	return ea.ToFilecoinAddress()
}
