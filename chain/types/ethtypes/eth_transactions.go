package ethtypes

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/filecoin-project/lotus/build"
	mathbig "math/big"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/types"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

type EthereumTransaction interface {
	ToUnsignedMessage(from address.Address) (*types.Message, error)
	ToSignedMessage() (*types.SignedMessage, error)
	ToRlpUnsignedMsg() ([]byte, error)
	TxHash() (EthHash, error)
	ToRlpSignedMsg() ([]byte, error)
	packTxFields() ([]interface{}, error)
	Signature() (*typescrypto.Signature, error)
	Sender() (address.Address, error)
	SetEthSignatureValues(sig typescrypto.Signature) error
	VerifiableSignature(sig []byte) ([]byte, error)
	ToEthTx(*types.SignedMessage) (EthTx, error)
}

type EthTx struct {
	ChainID              *EthUint64  `json:"chainId,omitempty"`
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
	MaxFeePerGas         *EthBigInt  `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas *EthBigInt  `json:"maxPriorityFeePerGas,omitempty"`
	GasPrice             EthBigInt   `json:"gasPrice"`
	AccessList           []EthHash   `json:"accessList,omitempty"`
	V                    EthBigInt   `json:"v"`
	R                    EthBigInt   `json:"r"`
	S                    EthBigInt   `json:"s"`
}

func (tx *EthTx) GasFeeCap() EthBigInt {
	return tx.GasPrice
}

func (tx *EthTx) GasPremium() EthBigInt {
	if tx.MaxPriorityFeePerGas == nil {
		return tx.GasPrice
	}
	return *tx.MaxPriorityFeePerGas
}

func EthereumTransactionFromSignedEthMessage(smsg *types.SignedMessage) (EthereumTransaction, error) {
	// The from address is always an f410f address, never an ID or other address.
	if !IsEthAddress(smsg.Message.From) {
		return nil, xerrors.Errorf("sender must be an eth account, was %s", smsg.Message.From)
	}

	// Probably redundant, but we might as well check.
	if smsg.Signature.Type != typescrypto.SigTypeDelegated {
		return nil, xerrors.Errorf("signature is not delegated type, is type: %d", smsg.Signature.Type)
	}

	_, err := EthAddressFromFilecoinAddress(smsg.Message.From)
	if err != nil {
		// This should be impossible as we've already asserted that we have an EthAddress
		// sender...
		return nil, xerrors.Errorf("sender was not an eth account")
	}

	params, to, err := parseMessageParamsAndReceipient(&smsg.Message)
	if err != nil {
		return nil, err
	}

	msg := smsg.Message

	if msg.Version != 0 {
		return nil, xerrors.Errorf("unsupported msg version: %d", msg.Version)
	}

	switch len(smsg.Signature.Data) {
	case 66:
		if smsg.Signature.Data[0] != LegacyHomesteadEthTxPrefix {
			return nil, fmt.Errorf("unsupported legacy transaction; first byte of signature is %d",
				smsg.Signature.Data[0])
		}
		tx := EthLegacyHomesteadTxArgs{
			Nonce:    int(msg.Nonce),
			To:       to,
			Value:    msg.Value,
			Input:    params,
			GasPrice: msg.GasFeeCap,
			GasLimit: int(msg.GasLimit),
		}
		tx.SetEthSignatureValues(smsg.Signature)
		fmt.Println("FINISHED SETTING ETH SIGNATURE VALUES")
		return &tx, nil
	case 65:
		tx := Eth1559TxArgs{
			ChainID:              build.Eip155ChainId,
			Nonce:                int(msg.Nonce),
			To:                   to,
			Value:                msg.Value,
			Input:                params,
			MaxFeePerGas:         msg.GasFeeCap,
			MaxPriorityFeePerGas: msg.GasPremium,
			GasLimit:             int(msg.GasLimit),
		}
		tx.SetEthSignatureValues(smsg.Signature)
		return &tx, nil
	default:
		return nil, fmt.Errorf("unsupported signature length: %d", len(smsg.Signature.Data))
	}
}

func ParseEthTx(data []byte) (EthereumTransaction, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	if data[0] > 0x7f {
		return parseLegacyHomesteadTx(data)
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

type methodInfo struct {
	to     address.Address
	method abi.MethodNum
	params []byte
}

func filecoin_method_info(recipient *EthAddress, input []byte) (*methodInfo, error) {
	var err error
	var params []byte
	if len(input) > 0 {
		buf := new(bytes.Buffer)
		if err = cbg.WriteByteArray(buf, input); err != nil {
			return nil, xerrors.Errorf("failed to write input args: %w", err)
		}
		params = buf.Bytes()
	}

	var to address.Address
	var method abi.MethodNum
	// nil indicates the EAM, only CreateExternal is allowed
	if recipient == nil {
		method = builtintypes.MethodsEAM.CreateExternal
		to = builtintypes.EthereumAddressManagerActorAddr
	} else {
		method = builtintypes.MethodsEVM.InvokeContract
		to, err = recipient.ToFilecoinAddress()
		if err != nil {
			return nil, xerrors.Errorf("failed to convert To into filecoin addr: %w", err)
		}
	}

	return &methodInfo{
		to:     to,
		method: method,
		params: params,
	}, nil
}

func packSigFields(v, r, s big.Int) ([]interface{}, error) {
	rr, err := formatBigInt(r)
	if err != nil {
		return nil, err
	}

	ss, err := formatBigInt(s)
	if err != nil {
		return nil, err
	}

	vv, err := formatBigInt(v)
	if err != nil {
		return nil, err
	}

	res := []interface{}{vv, rr, ss}
	return res, nil
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
	addr, err := CastEthAddress(b)
	if err != nil {
		return nil, err
	}
	return &addr, nil
}

func toSignedMessageCommon(tx EthereumTransaction) (*types.SignedMessage, error) {
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

func parseLegacyHomesteadTx(data []byte) (*EthLegacyHomesteadTxArgs, error) {
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

	return &EthLegacyHomesteadTxArgs{
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

func parseMessageParamsAndReceipient(msg *types.Message) ([]byte, *EthAddress, error) {
	var params []byte
	var to *EthAddress

	if len(msg.Params) > 0 {
		paramsReader := bytes.NewReader(msg.Params)
		var err error
		params, err = cbg.ReadByteArray(paramsReader, uint64(len(msg.Params)))
		if err != nil {
			return nil, nil, xerrors.Errorf("failed to read params byte array: %w", err)
		}
		if paramsReader.Len() != 0 {
			return nil, nil, xerrors.Errorf("extra data found in params")
		}
		if len(params) == 0 {
			return nil, nil, xerrors.Errorf("non-empty params encode empty byte array")
		}
	}

	if msg.To == builtintypes.EthereumAddressManagerActorAddr {
		if msg.Method != builtintypes.MethodsEAM.CreateExternal {
			return nil, nil, fmt.Errorf("unsupported EAM method")
		}
	} else if msg.Method == builtintypes.MethodsEVM.InvokeContract {
		addr, err := EthAddressFromFilecoinAddress(msg.To)
		if err != nil {
			return nil, nil, err
		}
		to = &addr
	} else {
		return nil, nil,
			xerrors.Errorf("invalid methodnum %d: only allowed method is InvokeContract(%d)",
				msg.Method, builtintypes.MethodsEVM.InvokeContract)
	}

	return params, to, nil
}
