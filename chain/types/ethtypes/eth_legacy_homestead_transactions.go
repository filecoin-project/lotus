package ethtypes

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	gocrypto "github.com/filecoin-project/go-crypto"
	"github.com/filecoin-project/go-state-types/big"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/types"
	"golang.org/x/crypto/sha3"
	"golang.org/x/xerrors"
)

// define a one byte prefix for legacy homestead eth transactions
const LegacyHomesteadEthTxPrefix = 0x01

var _ EthereumTransaction = (*EthLegacyHomesteadTxArgs)(nil)

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
		// This should be impossible as we've already asserted that we have an EthAddress
		// sender...
		return EthTx{}, xerrors.Errorf("sender was not an eth account")
	}
	hash, err := tx.TxHash()
	if err != nil {
		return EthTx{}, err
	}

	gasPrice := EthBigInt(tx.GasPrice)

	ethTx := EthTx{
		ChainID:  0x00,
		Type:     0x00,
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

func (tx *EthLegacyHomesteadTxArgs) ToUnsignedMessage(from address.Address) (*types.Message, error) {
	mi, err := filecoin_method_info(tx.To, tx.Input)
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

func (tx *EthLegacyHomesteadTxArgs) VerifiableSignature(sig []byte) ([]byte, error) {
	if len(sig) != 66 {
		return nil, fmt.Errorf("signature should be 66 bytes long, but got %d bytes", len(sig))
	}
	if sig[0] != LegacyHomesteadEthTxPrefix {
		return nil, fmt.Errorf("signature prefix should be 0x01, but got %x", sig[0])
	}

	fmt.Println("sig here is", sig)

	sig = sig[1:]

	// legacy transactions have a `V` value of 27 or 28 but new transactions have a `V` value of 0 or 1
	vValue := big.NewInt(0).SetBytes(sig[64:])
	if vValue.Uint64() != 27 && vValue.Uint64() != 28 {
		return nil, fmt.Errorf("v value is not 27 or 28 for legacy transaction")
	}
	vValue_ := big.Sub(big.NewFromGo(vValue), big.NewInt(27))
	// we ignore the first byte of the signature data because it is the prefix for legacy txns
	sig[64] = byte(vValue_.Uint64())

	return sig, nil
}

func (tx *EthLegacyHomesteadTxArgs) ToSignedMessage() (*types.SignedMessage, error) {
	return toSignedMessageCommon(tx)
}

func (tx *EthLegacyHomesteadTxArgs) ToRlpUnsignedMsg() ([]byte, error) {
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

func (tx *EthLegacyHomesteadTxArgs) TxHash() (EthHash, error) {
	rlp, err := tx.ToRlpSignedMsg()
	if err != nil {
		return EthHash{}, err
	}
	return EthHashFromTxBytes(rlp), nil
}

func (tx *EthLegacyHomesteadTxArgs) ToRlpSignedMsg() ([]byte, error) {
	packed1, err := tx.packTxFields()
	if err != nil {
		return nil, err
	}

	packed2, err := packSigFields(tx.V, tx.R, tx.S)
	if err != nil {
		return nil, err
	}

	encoded, err := EncodeRLP(append(packed1, packed2...))
	if err != nil {
		return nil, err
	}
	return encoded, nil
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
	if len(sig) != 65 {
		return nil, fmt.Errorf("signature is not 65 bytes")
	}

	// pre-pend a one byte marker so nodes know that this is a legacy transaction
	sig = append([]byte{LegacyHomesteadEthTxPrefix}, sig...)

	return &typescrypto.Signature{
		Type: typescrypto.SigTypeDelegated, Data: sig,
	}, nil
}

func (tx *EthLegacyHomesteadTxArgs) Sender() (address.Address, error) {
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

	fmt.Println("sig: ", sig.Data)

	sigData, err := tx.VerifiableSignature(sig.Data)
	if err != nil {
		return address.Undef, err
	}

	fmt.Println("sigData: ", sigData)

	pubk, err := gocrypto.EcRecover(hash, sigData)
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

func (tx *EthLegacyHomesteadTxArgs) SetEthSignatureValues(sig typescrypto.Signature) error {
	if sig.Type != typescrypto.SigTypeDelegated {
		return fmt.Errorf("RecoverSignature only supports Delegated signature")
	}

	if len(sig.Data) != 66 {
		return fmt.Errorf("signature should be 66 bytes long, but got %d bytes", len(sig.Data))
	}

	// ignore the first byte of the tx

	r_, err := parseBigInt(sig.Data[1:33])
	if err != nil {
		return fmt.Errorf("cannot parse r into EthBigInt")
	}

	s_, err := parseBigInt(sig.Data[33:65])
	if err != nil {
		return fmt.Errorf("cannot parse s into EthBigInt")
	}

	v_, err := parseBigInt([]byte{sig.Data[65]})
	if err != nil {
		return fmt.Errorf("cannot parse v into EthBigInt")
	}
	tx.R = r_
	tx.S = s_
	tx.V = v_
	fmt.Println("tx.V: ", tx.V)
	fmt.Println("tx.R: ", tx.R)
	fmt.Println("tx.S: ", tx.S)
	return nil
}
