package ethtypes

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ EthTransaction = (*Eth1559TxArgs)(nil)

type Eth1559TxArgs struct {
	ChainID              int         `json:"chainId"`
	Nonce                int         `json:"nonce"`
	To                   *EthAddress `json:"to"`
	Value                big.Int     `json:"value"`
	MaxFeePerGas         big.Int     `json:"maxFeePerGas"`
	MaxPriorityFeePerGas big.Int     `json:"maxPriorityFeePerGas"`
	GasLimit             int         `json:"gasLimit"`
	Input                []byte      `json:"input"`
	V                    big.Int     `json:"v"`
	R                    big.Int     `json:"r"`
	S                    big.Int     `json:"s"`
}

func (tx *Eth1559TxArgs) ToUnsignedFilecoinMessage(from address.Address) (*types.Message, error) {
	if tx.ChainID != buildconstants.Eip155ChainId {
		return nil, fmt.Errorf("invalid chain id: %d", tx.ChainID)
	}
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
		GasFeeCap:  tx.MaxFeePerGas,
		GasPremium: tx.MaxPriorityFeePerGas,
		Method:     mi.method,
		Params:     mi.params,
	}, nil
}

func (tx *Eth1559TxArgs) ToRlpUnsignedMsg() ([]byte, error) {
	encoded, err := toRlpUnsignedMsg(tx)
	if err != nil {
		return nil, err
	}
	return append([]byte{EIP1559TxType}, encoded...), nil
}

func (tx *Eth1559TxArgs) TxHash() (EthHash, error) {
	rlp, err := tx.ToRlpSignedMsg()
	if err != nil {
		return EmptyEthHash, err
	}

	return EthHashFromTxBytes(rlp), nil
}

func (tx *Eth1559TxArgs) ToRlpSignedMsg() ([]byte, error) {
	encoded, err := toRlpSignedMsg(tx, tx.V, tx.R, tx.S)
	if err != nil {
		return nil, err
	}
	return append([]byte{EIP1559TxType}, encoded...), nil
}

func (tx *Eth1559TxArgs) Signature() (*typescrypto.Signature, error) {
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
		return nil, xerrors.Errorf("signature is not 65 bytes")
	}
	return &typescrypto.Signature{
		Type: typescrypto.SigTypeDelegated, Data: sig,
	}, nil
}

func (tx *Eth1559TxArgs) Sender() (address.Address, error) {
	return sender(tx)
}

func (tx *Eth1559TxArgs) Type() int {
	return EIP1559TxType
}

func (tx *Eth1559TxArgs) ToVerifiableSignature(sig []byte) ([]byte, error) {
	return sig, nil
}

func (tx *Eth1559TxArgs) ToEthTx(smsg *types.SignedMessage) (EthTx, error) {
	from, err := EthAddressFromFilecoinAddress(smsg.Message.From)
	if err != nil {
		return EthTx{}, xerrors.Errorf("sender was not an eth account")
	}
	hash, err := tx.TxHash()
	if err != nil {
		return EthTx{}, err
	}
	gasFeeCap := EthBigInt(tx.MaxFeePerGas)
	gasPremium := EthBigInt(tx.MaxPriorityFeePerGas)

	ethTx := EthTx{
		ChainID:              EthUint64(buildconstants.Eip155ChainId),
		Type:                 EIP1559TxType,
		Nonce:                EthUint64(tx.Nonce),
		Hash:                 hash,
		To:                   tx.To,
		Value:                EthBigInt(tx.Value),
		Input:                tx.Input,
		Gas:                  EthUint64(tx.GasLimit),
		MaxFeePerGas:         &gasFeeCap,
		MaxPriorityFeePerGas: &gasPremium,
		From:                 from,
		R:                    EthBigInt(tx.R),
		S:                    EthBigInt(tx.S),
		V:                    EthBigInt(tx.V),
	}

	return ethTx, nil
}

func (tx *Eth1559TxArgs) InitialiseSignature(sig typescrypto.Signature) error {
	if sig.Type != typescrypto.SigTypeDelegated {
		return xerrors.Errorf("RecoverSignature only supports Delegated signature")
	}

	if len(sig.Data) != EthEIP1559TxSignatureLen {
		return xerrors.Errorf("signature should be 65 bytes long, but got %d bytes", len(sig.Data))
	}

	r_, err := parseBigInt(sig.Data[0:32])
	if err != nil {
		return xerrors.Errorf("cannot parse r into EthBigInt")
	}

	s_, err := parseBigInt(sig.Data[32:64])
	if err != nil {
		return xerrors.Errorf("cannot parse s into EthBigInt")
	}

	v_, err := parseBigInt([]byte{sig.Data[64]})
	if err != nil {
		return xerrors.Errorf("cannot parse v into EthBigInt")
	}

	tx.R = r_
	tx.S = s_
	tx.V = v_

	return nil
}

func (tx *Eth1559TxArgs) packTxFields() ([]interface{}, error) {
	chainId, err := formatInt(tx.ChainID)
	if err != nil {
		return nil, err
	}

	nonce, err := formatInt(tx.Nonce)
	if err != nil {
		return nil, err
	}

	maxPriorityFeePerGas, err := formatBigInt(tx.MaxPriorityFeePerGas)
	if err != nil {
		return nil, err
	}

	maxFeePerGas, err := formatBigInt(tx.MaxFeePerGas)
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
		chainId,
		nonce,
		maxPriorityFeePerGas,
		maxFeePerGas,
		gasLimit,
		formatEthAddr(tx.To),
		value,
		tx.Input,
		[]interface{}{}, // access list
	}
	return res, nil
}

func parseEip1559Tx(data []byte) (*Eth1559TxArgs, error) {
	if data[0] != EIP1559TxType {
		return nil, xerrors.Errorf("not an EIP-1559 transaction: first byte is not %d", EIP1559TxType)
	}

	d, err := DecodeRLP(data[1:])
	if err != nil {
		return nil, err
	}
	decoded, ok := d.([]interface{})
	if !ok {
		return nil, xerrors.Errorf("not an EIP-1559 transaction: decoded data is not a list")
	}

	if len(decoded) != 12 {
		return nil, xerrors.Errorf("not an EIP-1559 transaction: should have 12 elements in the rlp list")
	}

	chainId, err := parseInt(decoded[0])
	if err != nil {
		return nil, err
	}

	nonce, err := parseInt(decoded[1])
	if err != nil {
		return nil, err
	}

	maxPriorityFeePerGas, err := parseBigInt(decoded[2])
	if err != nil {
		return nil, err
	}

	maxFeePerGas, err := parseBigInt(decoded[3])
	if err != nil {
		return nil, err
	}

	gasLimit, err := parseInt(decoded[4])
	if err != nil {
		return nil, err
	}

	to, err := parseEthAddr(decoded[5])
	if err != nil {
		return nil, err
	}

	value, err := parseBigInt(decoded[6])
	if err != nil {
		return nil, err
	}

	input, err := parseBytes(decoded[7])
	if err != nil {
		return nil, err
	}

	accessList, ok := decoded[8].([]interface{})
	if !ok || (ok && len(accessList) != 0) {
		return nil, xerrors.Errorf("access list should be an empty list")
	}

	r, err := parseBigInt(decoded[10])
	if err != nil {
		return nil, err
	}

	s, err := parseBigInt(decoded[11])
	if err != nil {
		return nil, err
	}

	v, err := parseBigInt(decoded[9])
	if err != nil {
		return nil, err
	}

	// EIP-1559 and EIP-2930 transactions only support 0 or 1 for v
	// Legacy and EIP-155 transactions support other values
	// https://github.com/ethers-io/ethers.js/blob/56fabe987bb8c1e4891fdf1e5d3fe8a4c0471751/packages/transactions/src.ts/index.ts#L333
	if !v.Equals(big.NewInt(0)) && !v.Equals(big.NewInt(1)) {
		return nil, xerrors.Errorf("EIP-1559 transactions only support 0 or 1 for v")
	}

	args := Eth1559TxArgs{
		ChainID:              chainId,
		Nonce:                nonce,
		To:                   to,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
		MaxFeePerGas:         maxFeePerGas,
		GasLimit:             gasLimit,
		Value:                value,
		Input:                input,
		V:                    v,
		R:                    r,
		S:                    s,
	}
	return &args, nil
}

func Eth1559TxArgsFromUnsignedFilecoinMessage(msg *types.Message) (*Eth1559TxArgs, error) {
	if msg.Version != 0 {
		return nil, fmt.Errorf("unsupported msg version: %d", msg.Version)
	}

	params, to, err := getEthParamsAndRecipient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to get eth params and recipient: %w", err)
	}

	return &Eth1559TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Nonce:                int(msg.Nonce),
		To:                   to,
		Value:                msg.Value,
		Input:                params,
		MaxFeePerGas:         msg.GasFeeCap,
		MaxPriorityFeePerGas: msg.GasPremium,
		GasLimit:             int(msg.GasLimit),
	}, nil
}
