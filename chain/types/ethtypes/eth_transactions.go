package ethtypes

import (
	"bytes"
	"encoding/binary"
	"fmt"
	mathbig "math/big"

	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/go-address"
	gocrypto "github.com/filecoin-project/go-crypto"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

const Eip1559TxType = 2

type EthTx struct {
	ChainID              EthUint64   `json:"chainId"`
	Nonce                EthUint64   `json:"nonce"`
	Hash                 EthHash     `json:"hash"`
	BlockHash            *EthHash    `json:"blockHash"`
	BlockNumber          *EthUint64  `json:"blockNumber"`
	TransactionIndex     *EthUint64  `json:"transactionIndex"`
	From                 EthAddress  `json:"from"`
	To                   *EthAddress `json:"to"`
	Value                EthBigInt   `json:"value"`
	Type                 EthUint64   `json:"type"`
	Input                EthBytes    `json:"input"`
	Gas                  EthUint64   `json:"gas"`
	MaxFeePerGas         EthBigInt   `json:"maxFeePerGas"`
	MaxPriorityFeePerGas EthBigInt   `json:"maxPriorityFeePerGas"`
	V                    EthBigInt   `json:"v"`
	R                    EthBigInt   `json:"r"`
	S                    EthBigInt   `json:"s"`
}

type EthTxArgs struct {
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

func NewEthTxArgsFromMessage(msg *types.Message) (EthTxArgs, error) {
	var (
		to            *EthAddress
		decodedParams []byte
		paramsReader  = bytes.NewReader(msg.Params)
	)

	if msg.To == builtintypes.EthereumAddressManagerActorAddr {
		switch msg.Method {
		case builtintypes.MethodsEAM.Create:
			var create eam.CreateParams
			if err := create.UnmarshalCBOR(paramsReader); err != nil {
				return EthTxArgs{}, err
			}
			decodedParams = create.Initcode
		case builtintypes.MethodsEAM.Create2:
			var create2 eam.Create2Params
			if err := create2.UnmarshalCBOR(paramsReader); err != nil {
				return EthTxArgs{}, err
			}
			decodedParams = create2.Initcode
		default:
			return EthTxArgs{}, fmt.Errorf("unsupported EAM method")
		}
	} else {
		addr, err := NewEthAddressFromFilecoinAddress(msg.To)
		if err != nil {
			return EthTxArgs{}, err
		}
		to = &addr

		if len(msg.Params) > 0 {
			params, err := cbg.ReadByteArray(paramsReader, uint64(len(msg.Params)))
			if err != nil {
				return EthTxArgs{}, err
			}
			decodedParams = params
		}
	}

	return EthTxArgs{
		ChainID:              build.Eip155ChainId,
		Nonce:                int(msg.Nonce),
		To:                   to,
		Value:                msg.Value,
		Input:                decodedParams,
		MaxFeePerGas:         msg.GasFeeCap,
		MaxPriorityFeePerGas: msg.GasPremium,
		GasLimit:             int(msg.GasLimit),
	}, nil
}

func (tx *EthTxArgs) ToSignedMessage() (*types.SignedMessage, error) {
	from, err := tx.Sender()
	if err != nil {
		return nil, err
	}

	var to address.Address
	var params []byte

	if len(tx.To) == 0 && len(tx.Input) == 0 {
		return nil, fmt.Errorf("to and input cannot both be empty")
	}

	var method abi.MethodNum
	if tx.To == nil {
		// TODO unify with applyEvmMsg

		// this is a contract creation
		to = builtintypes.EthereumAddressManagerActorAddr

		params2, err := actors.SerializeParams(&eam.CreateParams{
			Initcode: tx.Input,
			Nonce:    uint64(tx.Nonce),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to serialize Create params: %w", err)
		}
		params = params2
		method = builtintypes.MethodsEAM.Create
	} else {
		addr, err := tx.To.ToFilecoinAddress()
		if err != nil {
			return nil, err
		}
		to = addr

		if len(tx.Input) > 0 {
			var buf bytes.Buffer
			if err := cbg.WriteByteArray(&buf, tx.Input); err != nil {
				return nil, fmt.Errorf("failed to encode tx input into a cbor byte-string")
			}
			params = buf.Bytes()
			method = builtintypes.MethodsEVM.InvokeContract
		} else {
			method = builtintypes.MethodSend
		}
	}

	msg := &types.Message{
		Nonce:      uint64(tx.Nonce),
		From:       from,
		To:         to,
		Value:      tx.Value,
		Method:     method,
		Params:     params,
		GasLimit:   int64(tx.GasLimit),
		GasFeeCap:  tx.MaxFeePerGas,
		GasPremium: tx.MaxPriorityFeePerGas,
	}

	sig, err := tx.Signature()
	if err != nil {
		return nil, err
	}

	signedMsg := types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}
	return &signedMsg, nil

}

func (tx *EthTxArgs) HashedOriginalRlpMsg() ([]byte, error) {
	msg, err := tx.ToRlpUnsignedMsg()
	if err != nil {
		return nil, err
	}

	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(msg)
	hash := hasher.Sum(nil)
	return hash, nil
}

func (tx *EthTxArgs) ToRlpUnsignedMsg() ([]byte, error) {
	packed, err := tx.packTxFields()
	if err != nil {
		return nil, err
	}

	encoded, err := EncodeRLP(packed)
	if err != nil {
		return nil, err
	}
	return append([]byte{0x02}, encoded...), nil
}

func (tx *EthTxArgs) ToRlpSignedMsg() ([]byte, error) {
	packed1, err := tx.packTxFields()
	if err != nil {
		return nil, err
	}

	packed2, err := tx.packSigFields()
	if err != nil {
		return nil, err
	}

	encoded, err := EncodeRLP(append(packed1, packed2...))
	if err != nil {
		return nil, err
	}
	return append([]byte{0x02}, encoded...), nil
}

func (tx *EthTxArgs) packTxFields() ([]interface{}, error) {
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

func (tx *EthTxArgs) packSigFields() ([]interface{}, error) {
	r, err := formatBigInt(tx.R)
	if err != nil {
		return nil, err
	}

	s, err := formatBigInt(tx.S)
	if err != nil {
		return nil, err
	}

	v, err := formatBigInt(tx.V)
	if err != nil {
		return nil, err
	}

	res := []interface{}{v, r, s}
	return res, nil
}

func (tx *EthTxArgs) Signature() (*typescrypto.Signature, error) {
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
	return &typescrypto.Signature{
		Type: typescrypto.SigTypeDelegated, Data: sig,
	}, nil
}

func (tx *EthTxArgs) Sender() (address.Address, error) {
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

	pubk, err := gocrypto.EcRecover(hash, sig.Data)
	if err != nil {
		return address.Undef, err
	}

	ethAddr, err := NewEthAddressFromPubKey(pubk)
	if err != nil {
		return address.Undef, err
	}

	ea, err := NewEthAddressFromBytes(ethAddr)
	if err != nil {
		return address.Undef, err
	}

	return ea.ToFilecoinAddress()
}

func RecoverSignature(sig typescrypto.Signature) (r, s, v EthBigInt, err error) {
	if sig.Type != typescrypto.SigTypeDelegated {
		return EthBigIntZero, EthBigIntZero, EthBigIntZero, fmt.Errorf("RecoverSignature only supports Delegated signature")
	}

	if len(sig.Data) != 65 {
		return EthBigIntZero, EthBigIntZero, EthBigIntZero, fmt.Errorf("signature should be 65 bytes long, but got %d bytes", len(sig.Data))
	}

	r_, err := parseBigInt(sig.Data[0:32])
	if err != nil {
		return EthBigIntZero, EthBigIntZero, EthBigIntZero, fmt.Errorf("cannot parse r into EthBigInt")
	}

	s_, err := parseBigInt(sig.Data[32:64])
	if err != nil {
		return EthBigIntZero, EthBigIntZero, EthBigIntZero, fmt.Errorf("cannot parse s into EthBigInt")
	}

	v_, err := parseBigInt([]byte{sig.Data[64]})
	if err != nil {
		return EthBigIntZero, EthBigIntZero, EthBigIntZero, fmt.Errorf("cannot parse v into EthBigInt")
	}

	return EthBigInt(r_), EthBigInt(s_), EthBigInt(v_), nil
}

func parseEip1559Tx(data []byte) (*EthTxArgs, error) {
	if data[0] != 2 {
		return nil, fmt.Errorf("not an EIP-1559 transaction: first byte is not 2")
	}

	d, err := DecodeRLP(data[1:])
	if err != nil {
		return nil, err
	}
	decoded, ok := d.([]interface{})
	if !ok {
		return nil, fmt.Errorf("not an EIP-1559 transaction: decoded data is not a list")
	}

	if len(decoded) != 9 && len(decoded) != 12 {
		return nil, fmt.Errorf("not an EIP-1559 transaction: should have 6 or 9 elements in the list")
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
		return nil, fmt.Errorf("access list should be an empty list")
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
		return nil, fmt.Errorf("EIP-1559 transactions only support 0 or 1 for v")
	}

	args := EthTxArgs{
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

func ParseEthTxArgs(data []byte) (*EthTxArgs, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	if data[0] > 0x7f {
		// legacy transaction
		return nil, fmt.Errorf("legacy transaction is not supported")
	}

	if data[0] == 1 {
		// EIP-2930
		return nil, fmt.Errorf("EIP-2930 transaction is not supported")
	}

	if data[0] == Eip1559TxType {
		// EIP-1559
		return parseEip1559Tx(data)
	}

	return nil, fmt.Errorf("unsupported transaction type")
}

func padLeadingZeros(data []byte, length int) []byte {
	if len(data) >= length {
		return data
	}
	zeros := make([]byte, length-len(data))
	return append(zeros, data...)
}

func removeLeadingZeros(data []byte) []byte {
	firstNonZeroIndex := len(data)
	for i, b := range data {
		if b > 0 {
			firstNonZeroIndex = i
			break
		}
	}
	return data[firstNonZeroIndex:]
}

func formatInt(val int) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, int64(val))
	if err != nil {
		return nil, err
	}
	return removeLeadingZeros(buf.Bytes()), nil
}

func formatEthAddr(addr *EthAddress) []byte {
	if addr == nil {
		return nil
	}
	return addr[:]
}

func formatBigInt(val big.Int) ([]byte, error) {
	b, err := val.Bytes()
	if err != nil {
		return nil, err
	}
	return removeLeadingZeros(b), nil
}

func parseInt(v interface{}) (int, error) {
	data, ok := v.([]byte)
	if !ok {
		return 0, fmt.Errorf("cannot parse interface to int: input is not a byte array")
	}
	if len(data) == 0 {
		return 0, nil
	}
	if len(data) > 8 {
		return 0, fmt.Errorf("cannot parse interface to int: length is more than 8 bytes")
	}
	var value int64
	r := bytes.NewReader(append(make([]byte, 8-len(data)), data...))
	if err := binary.Read(r, binary.BigEndian, &value); err != nil {
		return 0, fmt.Errorf("cannot parse interface to EthUint64: %w", err)
	}
	return int(value), nil
}

func parseBigInt(v interface{}) (big.Int, error) {
	data, ok := v.([]byte)
	if !ok {
		return big.Zero(), fmt.Errorf("cannot parse interface to big.Int: input is not a byte array")
	}
	if len(data) == 0 {
		return big.Zero(), nil
	}
	var b mathbig.Int
	b.SetBytes(data)
	return big.NewFromGo(&b), nil
}

func parseBytes(v interface{}) ([]byte, error) {
	val, ok := v.([]byte)
	if !ok {
		return nil, fmt.Errorf("cannot parse interface into bytes: input is not a byte array")
	}
	return val, nil
}

func parseEthAddr(v interface{}) (*EthAddress, error) {
	b, err := parseBytes(v)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, nil
	}
	addr, err := NewEthAddressFromBytes(b)
	if err != nil {
		return nil, err
	}
	return &addr, nil
}
