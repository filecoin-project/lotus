package ethtypes

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ EthTransaction = (*EthLegacy155TxArgs)(nil)

// EthLegacy155TxArgs is a legacy Ethereum transaction that uses the EIP-155 chain replay protection mechanism
// by incorporating the chainId in the signature.
// See how the `V` value in the signature is derived from the chainId at
// https://github.com/ethereum/go-ethereum/blob/86a1f0c39494c8f5caddf6bd9fbddd4bdfa944fd/core/types/transaction_signing.go#L424
// For EthLegacy155TxArgs, the digest that is used to create a signed transaction includes the `ChainID` but the serialised RLP transaction
// does not include the `ChainID` as an explicit field. Instead, the `ChainID` is included in the V value of the signature as mentioned above.
type EthLegacy155TxArgs struct {
	legacyTx *EthLegacyHomesteadTxArgs
}

func NewEthLegacy155TxArgs(tx *EthLegacyHomesteadTxArgs) *EthLegacy155TxArgs {
	return &EthLegacy155TxArgs{legacyTx: tx}
}

func (tx *EthLegacy155TxArgs) GetLegacyTx() *EthLegacyHomesteadTxArgs {
	return tx.legacyTx
}

func (tx *EthLegacy155TxArgs) ToEthTx(smsg *types.SignedMessage) (EthTx, error) {
	from, err := EthAddressFromFilecoinAddress(smsg.Message.From)
	if err != nil {
		return EthTx{}, fmt.Errorf("sender was not an eth account")
	}
	hash, err := tx.TxHash()
	if err != nil {
		return EthTx{}, fmt.Errorf("failed to get tx hash: %w", err)
	}

	gasPrice := EthBigInt(tx.legacyTx.GasPrice)
	ethTx := EthTx{
		ChainID:  buildconstants.Eip155ChainId,
		Type:     EthLegacyTxType,
		Nonce:    EthUint64(tx.legacyTx.Nonce),
		Hash:     hash,
		To:       tx.legacyTx.To,
		Value:    EthBigInt(tx.legacyTx.Value),
		Input:    tx.legacyTx.Input,
		Gas:      EthUint64(tx.legacyTx.GasLimit),
		GasPrice: &gasPrice,
		From:     from,
		R:        EthBigInt(tx.legacyTx.R),
		S:        EthBigInt(tx.legacyTx.S),
		V:        EthBigInt(tx.legacyTx.V),
	}

	return ethTx, nil
}

func (tx *EthLegacy155TxArgs) ToUnsignedFilecoinMessage(from address.Address) (*types.Message, error) {
	if err := validateEIP155ChainId(tx.legacyTx.V); err != nil {
		return nil, fmt.Errorf("failed to validate EIP155 chain id: %w", err)
	}
	return tx.legacyTx.ToUnsignedFilecoinMessage(from)
}

func (tx *EthLegacy155TxArgs) ToRlpUnsignedMsg() ([]byte, error) {
	return toRlpUnsignedMsg(tx)
}

func (tx *EthLegacy155TxArgs) TxHash() (EthHash, error) {
	encoded, err := tx.ToRawTxBytesSigned()
	if err != nil {
		return EthHash{}, fmt.Errorf("failed to encode rlp signed msg: %w", err)
	}

	return EthHashFromTxBytes(encoded), nil
}

func (tx *EthLegacy155TxArgs) ToRawTxBytesSigned() ([]byte, error) {
	packed1, err := tx.packTxFields()
	if err != nil {
		return nil, err
	}

	packed1 = packed1[:len(packed1)-3] // remove chainId, r and s as they are only used for signature verification

	packed2, err := packSigFields(tx.legacyTx.V, tx.legacyTx.R, tx.legacyTx.S)
	if err != nil {
		return nil, err
	}

	encoded, err := EncodeRLP(append(packed1, packed2...))
	if err != nil {
		return nil, fmt.Errorf("failed to encode rlp signed msg: %w", err)
	}
	return encoded, nil
}

func (tx *EthLegacy155TxArgs) ToRlpSignedMsg() ([]byte, error) {
	return toRlpSignedMsg(tx, tx.legacyTx.V, tx.legacyTx.R, tx.legacyTx.S)
}

func (tx *EthLegacy155TxArgs) Signature() (*typescrypto.Signature, error) {
	if err := validateEIP155ChainId(tx.legacyTx.V); err != nil {
		return nil, fmt.Errorf("failed to validate EIP155 chain id: %w", err)
	}
	r := tx.legacyTx.R.Int.Bytes()
	s := tx.legacyTx.S.Int.Bytes()
	v := tx.legacyTx.V.Int.Bytes()

	sig := append([]byte{}, padLeadingZeros(r, 32)...)
	sig = append(sig, padLeadingZeros(s, 32)...)
	sig = append(sig, v...)

	// prepend a one byte marker so nodes know that this is a legacy transaction
	sig = append([]byte{EthLegacy155TxSignaturePrefix}, sig...)

	if len(sig) != EthLegacy155TxSignatureLen0 && len(sig) != EthLegacy155TxSignatureLen1 {
		return nil, fmt.Errorf("signature is not %d OR %d bytes; it is %d bytes", EthLegacy155TxSignatureLen0, EthLegacy155TxSignatureLen1,
			len(sig))
	}

	return &typescrypto.Signature{
		Type: typescrypto.SigTypeDelegated, Data: sig,
	}, nil
}

func (tx *EthLegacy155TxArgs) Sender() (address.Address, error) {
	if err := validateEIP155ChainId(tx.legacyTx.V); err != nil {
		return address.Address{}, fmt.Errorf("failed to validate EIP155 chain id: %w", err)
	}
	return sender(tx)
}

func (tx *EthLegacy155TxArgs) Type() int {
	return EthLegacyTxType
}

var big8 = big.NewInt(8)

func (tx *EthLegacy155TxArgs) ToVerifiableSignature(sig []byte) ([]byte, error) {
	if len(sig) != EthLegacy155TxSignatureLen0 && len(sig) != EthLegacy155TxSignatureLen1 {
		return nil, fmt.Errorf("signature should be %d or %d bytes long but got %d bytes",
			EthLegacy155TxSignatureLen0, EthLegacy155TxSignatureLen1, len(sig))
	}
	if sig[0] != EthLegacy155TxSignaturePrefix {
		return nil, fmt.Errorf("expected signature prefix 0x%x, but got 0x%x", EthLegacy155TxSignaturePrefix, sig[0])
	}

	// Remove the prefix byte as it's only used for legacy transaction identification
	sig = sig[1:]

	// Extract the 'v' value from the signature
	vValue := big.NewFromGo(big.NewInt(0).SetBytes(sig[64:]))

	if err := validateEIP155ChainId(vValue); err != nil {
		return nil, fmt.Errorf("failed to validate EIP155 chain id: %w", err)
	}

	// See https://github.com/ethereum/go-ethereum/blob/86a1f0c39494c8f5caddf6bd9fbddd4bdfa944fd/core/types/transaction_signing.go#L424
	chainIdMul := big.Mul(big.NewIntUnsigned(buildconstants.Eip155ChainId), big.NewInt(2))
	vValue = big.Sub(vValue, chainIdMul)
	vValue = big.Sub(vValue, big8)

	// Adjust 'v' value for compatibility with new transactions: 27 -> 0, 28 -> 1
	if vValue.Equals(big.NewInt(27)) {
		sig[64] = 0
	} else if vValue.Equals(big.NewInt(28)) {
		sig[64] = 1
	} else {
		return nil, fmt.Errorf("invalid 'v' value: expected 27 or 28, got %d", vValue.Int64())
	}

	return sig[0:65], nil
}

func (tx *EthLegacy155TxArgs) InitialiseSignature(sig typescrypto.Signature) error {
	if sig.Type != typescrypto.SigTypeDelegated {
		return fmt.Errorf("RecoverSignature only supports Delegated signature")
	}

	if len(sig.Data) != EthLegacy155TxSignatureLen0 && len(sig.Data) != EthLegacy155TxSignatureLen1 {
		return fmt.Errorf("signature should be %d or %d bytes long, but got %d bytes", EthLegacy155TxSignatureLen0,
			EthLegacy155TxSignatureLen1, len(sig.Data))
	}

	if sig.Data[0] != EthLegacy155TxSignaturePrefix {
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

	v_, err := parseBigInt(sig.Data[65:])
	if err != nil {
		return fmt.Errorf("cannot parse v into EthBigInt: %w", err)
	}

	if err := validateEIP155ChainId(v_); err != nil {
		return fmt.Errorf("failed to validate EIP155 chain id: %w", err)
	}

	tx.legacyTx.R = r_
	tx.legacyTx.S = s_
	tx.legacyTx.V = v_
	return nil
}

func (tx *EthLegacy155TxArgs) packTxFields() ([]interface{}, error) {
	nonce, err := formatInt(tx.legacyTx.Nonce)
	if err != nil {
		return nil, err
	}

	// format gas price
	gasPrice, err := formatBigInt(tx.legacyTx.GasPrice)
	if err != nil {
		return nil, err
	}

	gasLimit, err := formatInt(tx.legacyTx.GasLimit)
	if err != nil {
		return nil, err
	}

	value, err := formatBigInt(tx.legacyTx.Value)
	if err != nil {
		return nil, err
	}

	chainIdBigInt := big.NewIntUnsigned(buildconstants.Eip155ChainId)
	chainId, err := formatBigInt(chainIdBigInt)
	if err != nil {
		return nil, err
	}

	r, err := formatInt(0)
	if err != nil {
		return nil, err
	}

	s, err := formatInt(0)
	if err != nil {
		return nil, err
	}

	res := []interface{}{
		nonce,
		gasPrice,
		gasLimit,
		formatEthAddr(tx.legacyTx.To),
		value,
		tx.legacyTx.Input,
		chainId,
		r, s,
	}
	return res, nil
}

func validateEIP155ChainId(v big.Int) error {
	chainId := deriveEIP155ChainId(v)
	if !chainId.Equals(big.NewIntUnsigned(buildconstants.Eip155ChainId)) {
		return fmt.Errorf("invalid chain id, expected %d, got %s", buildconstants.Eip155ChainId, chainId.String())
	}
	return nil
}

// deriveEIP155ChainId derives the chain id from the given v parameter
func deriveEIP155ChainId(v big.Int) big.Int {
	if big.BitLen(v) <= 64 {
		vUint64 := v.Uint64()
		if vUint64 == 27 || vUint64 == 28 {
			return big.NewInt(0)
		}
		return big.NewIntUnsigned((vUint64 - 35) / 2)
	}

	v = big.Sub(v, big.NewInt(35))
	return big.Div(v, big.NewInt(2))
}

func calcEIP155TxSignatureLen(chain uint64, v int) int {
	chainId := big.NewIntUnsigned(chain)
	vVal := big.Add(big.Mul(chainId, big.NewInt(2)), big.NewInt(int64(v)))
	vLen := len(vVal.Int.Bytes())

	// EthLegacyHomesteadTxSignatureLen includes the 1 byte legacy tx marker prefix and also 1 byte for the V value.
	// So we subtract 1 to not double count the length of the v value
	return EthLegacyHomesteadTxSignatureLen + vLen - 1
}
