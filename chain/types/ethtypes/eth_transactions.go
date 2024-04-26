package ethtypes

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	mathbig "math/big"

	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
)

const (
	// LegacyHomesteadEthTxSignaturePrefix defines the prefix byte used to identify the signature of a legacy Homestead Ethereum transaction.
	LegacyHomesteadEthTxSignaturePrefix = 0x01
	Eip1559TxType                       = 0x2

	ethLegacyHomesteadTxType    = 0x0
	ethLegacyHomesteadTxChainID = 0x0
)

// EthTransaction defines the interface for Ethereum-like transactions.
// It provides methods to convert transactions to various formats,
// retrieve transaction details, and manipulate transaction signatures.
type EthTransaction interface {
	Sender() (address.Address, error)
	Signature() (*typescrypto.Signature, error)
	InitialiseSignature(sig typescrypto.Signature) error
	ToUnsignedFilecoinMessage(from address.Address) (*types.Message, error)
	ToRlpUnsignedMsg() ([]byte, error)
	ToRlpSignedMsg() ([]byte, error)
	TxHash() (EthHash, error)
	ToVerifiableSignature(sig []byte) ([]byte, error)
	ToEthTx(*types.SignedMessage) (EthTx, error)
}

// EthTx represents an Ethereum transaction structure, encapsulating fields that align with the standard Ethereum transaction components.
// This structure can represent both EIP-1559 transactions and legacy Homestead transactions:
// - In EIP-1559 transactions, the `GasPrice` field is set to nil/empty.
// - In legacy Homestead transactions, the `GasPrice` field is populated to specify the fee per unit of gas, while the `MaxFeePerGas` and `MaxPriorityFeePerGas` fields are set to nil/empty.
// Additionally, both the `ChainID` and the `Type` fields are set to 0 in legacy Homestead transactions to differentiate them from EIP-1559 transactions.
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
	MaxFeePerGas         *EthBigInt  `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas *EthBigInt  `json:"maxPriorityFeePerGas,omitempty"`
	GasPrice             *EthBigInt  `json:"gasPrice,omitempty"`
	AccessList           []EthHash   `json:"accessList"`
	V                    EthBigInt   `json:"v"`
	R                    EthBigInt   `json:"r"`
	S                    EthBigInt   `json:"s"`
}

func (tx *EthTx) GasFeeCap() (EthBigInt, error) {
	if tx.GasPrice == nil && tx.MaxFeePerGas == nil {
		return EthBigInt{}, fmt.Errorf("gas fee cap is not set")
	}
	if tx.MaxFeePerGas != nil {
		return *tx.MaxFeePerGas, nil
	}
	return *tx.GasPrice, nil
}

func (tx *EthTx) GasPremium() (EthBigInt, error) {
	if tx.GasPrice == nil && tx.MaxPriorityFeePerGas == nil {
		return EthBigInt{}, fmt.Errorf("gas premium is not set")
	}

	if tx.MaxPriorityFeePerGas != nil {
		return *tx.MaxPriorityFeePerGas, nil
	}

	return *tx.GasPrice, nil
}

func EthTransactionFromSignedFilecoinMessage(smsg *types.SignedMessage) (EthTransaction, error) {
	if smsg == nil {
		return nil, errors.New("signed message is nil")
	}
	// Validate the sender's address format.
	if !IsEthAddress(smsg.Message.From) {
		return nil, fmt.Errorf("sender must be an eth account, was %s", smsg.Message.From)
	}

	// Ensure the signature type is delegated.
	if smsg.Signature.Type != typescrypto.SigTypeDelegated {
		return nil, fmt.Errorf("signature is not delegated type, is type: %d", smsg.Signature.Type)
	}

	// Convert Filecoin address to Ethereum address.
	_, err := EthAddressFromFilecoinAddress(smsg.Message.From)
	if err != nil {
		return nil, fmt.Errorf("sender was not an eth account")
	}

	// Extract Ethereum parameters and recipient from the message.
	params, to, err := getEthParamsAndRecipient(&smsg.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to parse input params and recipient: %w", err)
	}

	// Check for supported message version.
	if smsg.Message.Version != 0 {
		return nil, fmt.Errorf("unsupported msg version: %d", smsg.Message.Version)
	}

	// Process based on the signature data length.
	switch len(smsg.Signature.Data) {
	case 66: // Legacy Homestead transaction
		if smsg.Signature.Data[0] != LegacyHomesteadEthTxSignaturePrefix {
			return nil, fmt.Errorf("unsupported legacy transaction; first byte of signature is %d",
				smsg.Signature.Data[0])
		}
		tx := EthLegacyHomesteadTxArgs{
			Nonce:    int(smsg.Message.Nonce),
			To:       to,
			Value:    smsg.Message.Value,
			Input:    params,
			GasPrice: smsg.Message.GasFeeCap,
			GasLimit: int(smsg.Message.GasLimit),
		}
		if err := tx.InitialiseSignature(smsg.Signature); err != nil {
			return nil, fmt.Errorf("failed to initialise signature: %w", err)
		}
		return &tx, nil
	case 65: // EIP-1559 transaction
		tx := Eth1559TxArgs{
			ChainID:              build.Eip155ChainId,
			Nonce:                int(smsg.Message.Nonce),
			To:                   to,
			Value:                smsg.Message.Value,
			Input:                params,
			MaxFeePerGas:         smsg.Message.GasFeeCap,
			MaxPriorityFeePerGas: smsg.Message.GasPremium,
			GasLimit:             int(smsg.Message.GasLimit),
		}
		if err := tx.InitialiseSignature(smsg.Signature); err != nil {
			return nil, fmt.Errorf("failed to initialise signature: %w", err)
		}
		return &tx, nil
	default:
		return nil, fmt.Errorf("unsupported signature length: %d", len(smsg.Signature.Data))
	}
}

func ToSignedFilecoinMessage(tx EthTransaction) (*types.SignedMessage, error) {
	from, err := tx.Sender()
	if err != nil {
		return nil, fmt.Errorf("failed to calculate sender: %w", err)
	}

	unsignedMsg, err := tx.ToUnsignedFilecoinMessage(from)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to unsigned msg: %w", err)
	}

	siggy, err := tx.Signature()
	if err != nil {
		return nil, fmt.Errorf("failed to calculate signature: %w", err)
	}

	return &types.SignedMessage{
		Message:   *unsignedMsg,
		Signature: *siggy,
	}, nil
}

func ParseEthTransaction(data []byte) (EthTransaction, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	if data[0] > 0x7f {
		tx, err := parseLegacyHomesteadTx(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse legacy homestead transaction: %w", err)
		}
		return tx, nil
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

func getFilecoinMethodInfo(recipient *EthAddress, input []byte) (*methodInfo, error) {
	var params []byte
	if len(input) > 0 {
		buf := new(bytes.Buffer)
		if err := cbg.WriteByteArray(buf, input); err != nil {
			return nil, fmt.Errorf("failed to write input args: %w", err)
		}
		params = buf.Bytes()
	}

	var to address.Address
	var method abi.MethodNum

	if recipient == nil {
		// If recipient is nil, use Ethereum Address Manager Actor and CreateExternal method
		method = builtintypes.MethodsEAM.CreateExternal
		to = builtintypes.EthereumAddressManagerActorAddr
	} else {
		// Otherwise, use InvokeContract method and convert EthAddress to Filecoin address
		method = builtintypes.MethodsEVM.InvokeContract
		var err error
		to, err = recipient.ToFilecoinAddress()
		if err != nil {
			return nil, fmt.Errorf("failed to convert EthAddress to Filecoin address: %w", err)
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

func getEthParamsAndRecipient(msg *types.Message) (params []byte, to *EthAddress, err error) {
	if len(msg.Params) > 0 {
		paramsReader := bytes.NewReader(msg.Params)
		var err error
		params, err = cbg.ReadByteArray(paramsReader, uint64(len(msg.Params)))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read params byte array: %w", err)
		}
		if paramsReader.Len() != 0 {
			return nil, nil, fmt.Errorf("extra data found in params")
		}
		if len(params) == 0 {
			return nil, nil, fmt.Errorf("non-empty params encode empty byte array")
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
			fmt.Errorf("invalid methodnum %d: only allowed method is InvokeContract(%d)",
				msg.Method, builtintypes.MethodsEVM.InvokeContract)
	}

	return params, to, nil
}
