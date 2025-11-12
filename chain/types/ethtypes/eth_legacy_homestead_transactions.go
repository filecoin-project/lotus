package ethtypes

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
)

var _ EthTransaction = (*EthLegacyHomesteadTxArgs)(nil)

type EthLegacyHomesteadTxArgs struct {
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

func (tx *EthLegacyHomesteadTxArgs) ToEthTx(smsg *types.SignedMessage) (EthTx, error) {
	from, err := EthAddressFromFilecoinAddress(smsg.Message.From)
	if err != nil {
		return EthTx{}, fmt.Errorf("sender was not an eth account")
	}
	hash, err := tx.TxHash()
	if err != nil {
		return EthTx{}, fmt.Errorf("failed to get tx hash: %w", err)
	}

	gasPrice := EthBigInt(tx.GasPrice)
	ethTx := EthTx{
		ChainID:  EthLegacyHomesteadTxChainID,
		Type:     EthLegacyTxType,
		Nonce:    EthUint64(tx.Nonce),
		Hash:     hash,
		To:       tx.To,
		Value:    EthBigInt(tx.Value),
		Input:    tx.Input,
		Gas:      EthUint64(tx.GasLimit),
		GasPrice: &gasPrice,
		From:     from,
		R:        EthBigInt(tx.R),
		S:        EthBigInt(tx.S),
		V:        EthBigInt(tx.V),
	}

	return ethTx, nil
}

func (tx *EthLegacyHomesteadTxArgs) ToUnsignedFilecoinMessage(from address.Address) (*types.Message, error) {
	mi, err := getFilecoinMethodInfo(tx.To, tx.Input)
	if err != nil {
		return nil, xerrors.Errorf("failed to get method info: %w", err)
	}

	return &types.Message{
		Version:    0,
		To:         mi.to,
		From:       from,
		Nonce:      uint64(tx.Nonce),
		Value:      tx.Value,
		GasLimit:   int64(tx.GasLimit),
		GasFeeCap:  tx.GasPrice,
		GasPremium: tx.GasPrice,
		Method:     mi.method,
		Params:     mi.params,
	}, nil
}

func (tx *EthLegacyHomesteadTxArgs) ToVerifiableSignature(sig []byte) ([]byte, error) {
	if len(sig) != EthLegacyHomesteadTxSignatureLen {
		return nil, fmt.Errorf("signature should be %d bytes long (1 byte metadata, %d bytes sig data), but got %d bytes",
			EthLegacyHomesteadTxSignatureLen, EthLegacyHomesteadTxSignatureLen-1, len(sig))
	}
	if sig[0] != EthLegacyHomesteadTxSignaturePrefix {
		return nil, fmt.Errorf("expected signature prefix 0x%x, but got 0x%x", EthLegacyHomesteadTxSignaturePrefix, sig[0])
	}

	// Remove the prefix byte as it's only used for legacy transaction identification
	sig = sig[1:]

	// Extract the 'v' value from the signature, which is the last byte in Ethereum signatures
	vValue := big.NewFromGo(big.NewInt(0).SetBytes(sig[64:]))

	// Adjust 'v' value for compatibility with new transactions: 27 -> 0, 28 -> 1
	if vValue.Equals(big.NewInt(27)) {
		sig[64] = 0
	} else if vValue.Equals(big.NewInt(28)) {
		sig[64] = 1
	} else {
		return nil, fmt.Errorf("invalid 'v' value: expected 27 or 28, got %d", vValue.Int64())
	}

	return sig, nil
}

func (tx *EthLegacyHomesteadTxArgs) ToRlpUnsignedMsg() ([]byte, error) {
	return toRlpUnsignedMsg(tx)
}

func (tx *EthLegacyHomesteadTxArgs) TxHash() (EthHash, error) {
	rlp, err := tx.ToRlpSignedMsg()
	if err != nil {
		return EthHash{}, err
	}
	return EthHashFromTxBytes(rlp), nil
}

func (tx *EthLegacyHomesteadTxArgs) ToRlpSignedMsg() ([]byte, error) {
	return toRlpSignedMsg(tx, tx.V, tx.R, tx.S)
}

func (tx *EthLegacyHomesteadTxArgs) Signature() (*typescrypto.Signature, error) {
	// throw an error if the v value is not 27 or 28
	if !tx.V.Equals(big.NewInt(27)) && !tx.V.Equals(big.NewInt(28)) {
		return nil, fmt.Errorf("legacy homestead transactions only support 27 or 28 for v")
	}
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
	// prepend a one byte marker so nodes know that this is a legacy transaction
	sig = append([]byte{EthLegacyHomesteadTxSignaturePrefix}, sig...)

	if len(sig) != EthLegacyHomesteadTxSignatureLen {
		return nil, fmt.Errorf("signature is not %d bytes", EthLegacyHomesteadTxSignatureLen)
	}

	return &typescrypto.Signature{
		Type: typescrypto.SigTypeDelegated, Data: sig,
	}, nil
}

func (tx *EthLegacyHomesteadTxArgs) Sender() (address.Address, error) {
	return sender(tx)
}

func (tx *EthLegacyHomesteadTxArgs) Type() int {
	return EthLegacyTxType
}

func (tx *EthLegacyHomesteadTxArgs) InitialiseSignature(sig typescrypto.Signature) error {
	if sig.Type != typescrypto.SigTypeDelegated {
		return fmt.Errorf("RecoverSignature only supports Delegated signature")
	}

	if len(sig.Data) != EthLegacyHomesteadTxSignatureLen {
		return fmt.Errorf("signature should be %d bytes long, but got %d bytes", EthLegacyHomesteadTxSignatureLen, len(sig.Data))
	}

	if sig.Data[0] != EthLegacyHomesteadTxSignaturePrefix {
		return fmt.Errorf("expected signature prefix 0x01, but got 0x%x", sig.Data[0])
	}

	// ignore the first byte of the signature as it's only used for legacy transaction identification
	r_, err := parseBigInt(sig.Data[1:33])
	if err != nil {
		return fmt.Errorf("cannot parse r into EthBigInt: %w", err)
	}

	s_, err := parseBigInt(sig.Data[33:65])
	if err != nil {
		return fmt.Errorf("cannot parse s into EthBigInt: %w", err)
	}

	v_, err := parseBigInt([]byte{sig.Data[65]})
	if err != nil {
		return fmt.Errorf("cannot parse v into EthBigInt: %w", err)
	}

	if !v_.Equals(big.NewInt(27)) && !v_.Equals(big.NewInt(28)) {
		return fmt.Errorf("legacy homestead transactions only support 27 or 28 for v")
	}

	tx.R = r_
	tx.S = s_
	tx.V = v_
	return nil
}

func (tx *EthLegacyHomesteadTxArgs) packTxFields() ([]interface{}, error) {
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
