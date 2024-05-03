package ethtypes

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ EthTransaction = (*EthLegacy155TxArgs)(nil)

type EthLegacy155TxArgs struct {
	legacyTx *EthLegacyHomesteadTxArgs
}

// implement all interface methods
func (tx *EthLegacy155TxArgs) ToEthTx(smsg *types.SignedMessage) (EthTx, error) {
	ethTx, err := tx.legacyTx.ToEthTx(smsg)
	if err != nil {
		return EthTx{}, fmt.Errorf("failed to convert legacy tx to eth tx: %w", err)
	}
	if err := validateEIP155ChainId(tx.legacyTx.V); err != nil {
		return EthTx{}, fmt.Errorf("failed to validate EIP155 chain id: %w", err)
	}

	ethTx.ChainID = build.Eip155ChainId
	return ethTx, nil
}

func (tx *EthLegacy155TxArgs) ToUnsignedFilecoinMessage(from address.Address) (*types.Message, error) {
	if err := validateEIP155ChainId(tx.legacyTx.V); err != nil {
		return nil, fmt.Errorf("failed to validate EIP155 chain id: %w", err)
	}
	return tx.legacyTx.ToUnsignedFilecoinMessage(from)
}

func (tx *EthLegacy155TxArgs) ToRlpUnsignedMsg() ([]byte, error) {
	return tx.legacyTx.ToRlpUnsignedMsg()
}

func (tx *EthLegacy155TxArgs) TxHash() (EthHash, error) {
	return tx.legacyTx.TxHash()
}

func (tx *EthLegacy155TxArgs) ToRlpSignedMsg() ([]byte, error) {
	return tx.legacyTx.ToRlpSignedMsg()
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

	// pre-pend a one byte marker so nodes know that this is a legacy transaction
	sig = append([]byte{EthLegacy155TxSignaturePrefix}, sig...)

	if len(sig) != EthLegacy155TxSignatureLen {
		return nil, fmt.Errorf("signature is not %d bytes; it is %d bytes", EthLegacy155TxSignatureLen, len(sig))
	}

	return &typescrypto.Signature{
		Type: typescrypto.SigTypeDelegated, Data: sig,
	}, nil
}

func (tx *EthLegacy155TxArgs) Sender() (address.Address, error) {
	if err := validateEIP155ChainId(tx.legacyTx.V); err != nil {
		return address.Address{}, fmt.Errorf("failed to validate EIP155 chain id: %w", err)
	}
	return tx.legacyTx.Sender()
}

var big8 = big.NewInt(8)

func (tx *EthLegacy155TxArgs) ToVerifiableSignature(sig []byte) ([]byte, error) {
	if len(sig) != EthLegacy155TxSignatureLen {
		return nil, fmt.Errorf("signature should be %d bytes long (1 byte metadata, %d bytes sig data), but got %d bytes",
			EthLegacy155TxSignatureLen, EthLegacy155TxSignatureLen-1, len(sig))
	}
	if sig[0] != EthLegacy155TxSignaturePrefix {
		return nil, fmt.Errorf("expected signature prefix 0x%x, but got 0x%x", EthLegacy155TxSignaturePrefix, sig[0])
	}

	// Remove the prefix byte as it's only used for legacy transaction identification
	sig = sig[1:]

	// Extract the 'v' value from the signature, which is the last byte in Ethereum signatures
	vValue := big.NewFromGo(big.NewInt(0).SetBytes(sig[64:]))

	chainIdMul := big.Mul(big.NewIntUnsigned(build.Eip155ChainId), big.NewInt(2))
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

	return sig, nil

}

func (tx *EthLegacy155TxArgs) InitialiseSignature(sig typescrypto.Signature) error {
	if sig.Type != typescrypto.SigTypeDelegated {
		return fmt.Errorf("RecoverSignature only supports Delegated signature")
	}

	if len(sig.Data) != EthLegacy155TxSignatureLen {
		return fmt.Errorf("signature should be %d bytes long, but got %d bytes", EthLegacy155TxSignatureLen, len(sig.Data))
	}

	if sig.Data[0] != EthLegacy155TxSignaturePrefix {
		return fmt.Errorf("expected signature prefix 0x01, but got 0x%x", sig.Data[0])
	}

	// ignore the first byte of the tx as it's only used for legacy transaction identification
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
	return tx.legacyTx.packTxFields()
}

func validateEIP155ChainId(v big.Int) error {
	chainId := deriveEIP155ChainId(v)
	if !chainId.Equals(big.NewIntUnsigned(build.Eip155ChainId)) {
		return fmt.Errorf("invalid chain id, expected %d, got %s", build.Eip155ChainId, chainId.String())
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

func calcEIP155TxSignatureLen(chain uint64) int {
	chainId := big.NewIntUnsigned(chain)
	vVal := big.Add(big.Mul(chainId, big.NewInt(2)), big.NewInt(36))
	vLen := len(vVal.Int.Bytes())

	// EthLegacyHomesteadTxSignatureLen includes the 1 byte legacy tx marker prefix and also 1 byte for the V value.
	// So we subtract 1 to not double count the length of the v value
	return EthLegacyHomesteadTxSignatureLen + vLen - 1
}
