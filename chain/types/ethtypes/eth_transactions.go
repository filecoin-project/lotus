package ethtypes

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	mathbig "math/big"

	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/go-address"
	gocrypto "github.com/filecoin-project/go-crypto"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

const (
	EthLegacyTxType = 0x00
	EIP1559TxType   = 0x02
	// EIP-7702 typed transaction: Set Code for EOAs (authorization list)
	// https://eips.ethereum.org/EIPS/eip-7702
	EIP7702TxType = 0x04
)

const (
	EthEIP1559TxSignatureLen            = 65
	EthLegacyHomesteadTxSignatureLen    = 66
	EthLegacyHomesteadTxSignaturePrefix = 0x01
	EthLegacy155TxSignaturePrefix       = 0x02
	EthLegacyHomesteadTxChainID         = 0x00
)

var (
	EthLegacy155TxSignatureLen0 int
	EthLegacy155TxSignatureLen1 int
)

func init() {
	EthLegacy155TxSignatureLen0 = calcEIP155TxSignatureLen(buildconstants.Eip155ChainId, 35)
	EthLegacy155TxSignatureLen1 = calcEIP155TxSignatureLen(buildconstants.Eip155ChainId, 36)
}

// EthTransaction defines the interface for Ethereum-like transactions.
// It provides methods to convert transactions to various formats,
// retrieve transaction details, and manipulate transaction signatures.
type EthTransaction interface {
	Type() int
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
	// Present only for EIP-7702 (type 0x04) transactions
	AuthorizationList []EthAuthorization `json:"authorizationList,omitempty"`
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

	// Ensure the signature type is delegated.
	if smsg.Signature.Type != typescrypto.SigTypeDelegated {
		return nil, fmt.Errorf("signature is not delegated type, is type: %d", smsg.Signature.Type)
	}

	// Convert Filecoin address to Ethereum address.
	_, err := EthAddressFromFilecoinAddress(smsg.Message.From)
	if err != nil {
		return nil, fmt.Errorf("sender was not an eth account")
	}

	// Special-case: EthAccount.ApplyAndCall -> reconstruct a 0x04 tx view
	if smsg.Message.Method == abi.MethodNum(MethodHash("ApplyAndCall")) {
		if authz, err := strictDecodeApplyAndCallAuthorizations(smsg.Message.Params); err == nil && len(authz) > 0 {
			tx := &Eth7702TxArgs{
				ChainID:              buildconstants.Eip155ChainId,
				Nonce:                int(smsg.Message.Nonce),
				To:                   nil,
				Value:                smsg.Message.Value,
				MaxFeePerGas:         smsg.Message.GasFeeCap,
				MaxPriorityFeePerGas: smsg.Message.GasPremium,
				GasLimit:             int(smsg.Message.GasLimit),
				Input:                nil,
				AuthorizationList:    authz,
			}
			if err := tx.InitialiseSignature(smsg.Signature); err != nil {
				return nil, fmt.Errorf("failed to initialise signature: %w", err)
			}
			return tx, nil
		}
	}

	// Delegator route removed; routing is now via EthAccount.ApplyAndCall + VM intercept.

	// Extract Ethereum parameters and recipient from the message.
	params, to, err := getEthParamsAndRecipient(&smsg.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to parse input params and recipient: %w", err)
	}

	// Check for supported message version.
	if smsg.Message.Version != 0 {
		return nil, fmt.Errorf("unsupported msg version: %d", smsg.Message.Version)
	}

	// Determine the type of transaction based on the signature length
	switch len(smsg.Signature.Data) {
	case EthEIP1559TxSignatureLen:
		tx := Eth1559TxArgs{
			ChainID:              buildconstants.Eip155ChainId,
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

	case EthLegacyHomesteadTxSignatureLen, EthLegacy155TxSignatureLen0, EthLegacy155TxSignatureLen1:
		legacyTx := &EthLegacyHomesteadTxArgs{
			Nonce:    int(smsg.Message.Nonce),
			To:       to,
			Value:    smsg.Message.Value,
			Input:    params,
			GasPrice: smsg.Message.GasFeeCap,
			GasLimit: int(smsg.Message.GasLimit),
		}
		// Process based on the first byte of the signature
		switch smsg.Signature.Data[0] {
		case EthLegacyHomesteadTxSignaturePrefix:
			if err := legacyTx.InitialiseSignature(smsg.Signature); err != nil {
				return nil, fmt.Errorf("failed to initialise signature: %w", err)
			}
			return legacyTx, nil
		case EthLegacy155TxSignaturePrefix:
			tx := &EthLegacy155TxArgs{
				legacyTx: legacyTx,
			}
			if err := tx.InitialiseSignature(smsg.Signature); err != nil {
				return nil, fmt.Errorf("failed to initialise signature: %w", err)
			}
			return tx, nil
		default:
			return nil, fmt.Errorf("unsupported legacy transaction; first byte of signature is %d", smsg.Signature.Data[0])
		}

	default:
		return nil, fmt.Errorf("unsupported signature length")
	}
}

// strictDecodeApplyAndCallAuthorizations decodes the canonical ApplyAndCall params and returns
// the authorization list. It enforces:
// - top-level array length >= 1
// - first element is an array of 6-field tuples with exact field kinds
// Errors out early on malformed shapes.
func strictDecodeApplyAndCallAuthorizations(params []byte) ([]EthAuthorization, error) {
	r := cbg.NewCborReader(bytes.NewReader(params))
	maj, l, err := r.ReadHeader()
	if err != nil {
		return nil, err
	}
	if maj != cbg.MajArray || l < 1 {
		return nil, fmt.Errorf("applyandcall params must be array with >=1 elements")
	}
	maj0, l0, err := r.ReadHeader()
	if err != nil {
		return nil, err
	}
	if maj0 != cbg.MajArray {
		return nil, fmt.Errorf("authorizations must be array")
	}
	tmp := make([]EthAuthorization, 0, l0)
	for i := 0; i < int(l0); i++ {
		majT, tlen, err := r.ReadHeader()
		if err != nil || majT != cbg.MajArray || tlen != 6 {
			return nil, fmt.Errorf("authorization[%d]: not a 6-field tuple", i)
		}
		majF, v, err := r.ReadHeader()
		if err != nil || majF != cbg.MajUnsignedInt {
			return nil, fmt.Errorf("auth[%d]: bad chainId", i)
		}
		majF, blen, err := r.ReadHeader()
		if err != nil || majF != cbg.MajByteString || blen != 20 {
			return nil, fmt.Errorf("auth[%d]: bad address", i)
		}
		var ea EthAddress
		if _, err := r.Read(ea[:]); err != nil {
			return nil, fmt.Errorf("auth[%d]: bad address bytes", i)
		}
		majF, nv, err := r.ReadHeader()
		if err != nil || majF != cbg.MajUnsignedInt {
			return nil, fmt.Errorf("auth[%d]: bad nonce", i)
		}
		majF, yv, err := r.ReadHeader()
		if err != nil || majF != cbg.MajUnsignedInt {
			return nil, fmt.Errorf("auth[%d]: bad yParity", i)
		}
		majF, rbl, err := r.ReadHeader()
		if err != nil || majF != cbg.MajByteString {
			return nil, fmt.Errorf("auth[%d]: bad r", i)
		}
		rb := make([]byte, rbl)
		if _, err := r.Read(rb); err != nil {
			return nil, fmt.Errorf("auth[%d]: bad r bytes", i)
		}
		majF, sbl, err := r.ReadHeader()
		if err != nil || majF != cbg.MajByteString {
			return nil, fmt.Errorf("auth[%d]: bad s", i)
		}
		sb := make([]byte, sbl)
		if _, err := r.Read(sb); err != nil {
			return nil, fmt.Errorf("auth[%d]: bad s bytes", i)
		}
		tmp = append(tmp, EthAuthorization{
			ChainID: EthUint64(v),
			Address: ea,
			Nonce:   EthUint64(nv),
			YParity: uint8(yv),
			R:       EthBigInt(big.NewFromGo(new(mathbig.Int).SetBytes(rb))),
			S:       EthBigInt(big.NewFromGo(new(mathbig.Int).SetBytes(sb))),
		})
	}
	return tmp, nil
}

func ToSignedFilecoinMessage(tx EthTransaction) (*types.SignedMessage, error) {
	from, err := tx.Sender()
	if err != nil {
		return nil, fmt.Errorf("failed to calculate sender: %w", err)
	}

	var unsignedMsg *types.Message
	switch t := tx.(type) {
	case *Eth7702TxArgs:
		// Route 0x04 to atomic apply+call params
		unsignedMsg, err = t.ToUnsignedFilecoinMessageAtomic(from)
	default:
		unsignedMsg, err = tx.ToUnsignedFilecoinMessage(from)
	}
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

	switch data[0] {
	case 1:
		// EIP-2930
		return nil, fmt.Errorf("EIP-2930 transaction is not supported")
	case EIP1559TxType:
		// EIP-1559
		return parseEip1559Tx(data)
	case EIP7702TxType:
		// EIP-7702 (type 0x04)
		return parseEip7702Tx(data)
	default:
		if data[0] > 0x7f {
			tx, err := parseLegacyTx(data)
			if err != nil {
				return nil, fmt.Errorf("failed to parse legacy transaction: %w", err)
			}
			return tx, nil
		}
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

// formatUint64 encodes a uint64 as big-endian bytes without leading zeros (RLP-compatible).
func formatUint64(val uint64) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, val); err != nil {
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

// parseUint64 parses a big-endian encoded unsigned integer up to 8 bytes.
func parseUint64(v interface{}) (uint64, error) {
	data, ok := v.([]byte)
	if !ok {
		return 0, fmt.Errorf("cannot parse interface to uint64: input is not a byte array")
	}
	if len(data) == 0 {
		return 0, nil
	}
	if len(data) > 8 {
		return 0, fmt.Errorf("cannot parse interface to uint64: length is more than 8 bytes")
	}
	var value uint64
	r := bytes.NewReader(append(make([]byte, 8-len(data)), data...))
	if err := binary.Read(r, binary.BigEndian, &value); err != nil {
		return 0, fmt.Errorf("cannot parse interface to uint64: %w", err)
	}
	return value, nil
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
			fmt.Errorf("invalid methodnum %d: only allowed method is InvokeContract(%d) or CreateExternal(%d)",
				msg.Method, builtintypes.MethodsEVM.InvokeContract, builtintypes.MethodsEAM.CreateExternal)
	}

	return params, to, nil
}

func parseLegacyTx(data []byte) (EthTransaction, error) {
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

	tx := &EthLegacyHomesteadTxArgs{
		Nonce:    nonce,
		GasPrice: gasPrice,
		GasLimit: gasLimit,
		To:       to,
		Value:    value,
		Input:    input,
		V:        v,
		R:        r,
		S:        s,
	}

	chainId := deriveEIP155ChainId(v)
	if chainId.Equals(big.NewInt(0)) {
		// This is a legacy Homestead transaction
		if !v.Equals(big.NewInt(27)) && !v.Equals(big.NewInt(28)) {
			return nil, fmt.Errorf("legacy homestead transactions only support 27 or 28 for v, got %d", v.Uint64())
		}
		return tx, nil
	}

	// This is a EIP-155 transaction -> ensure chainID protection
	if err := validateEIP155ChainId(v); err != nil {
		return nil, fmt.Errorf("failed to validate EIP155 chain id: %w", err)
	}

	return &EthLegacy155TxArgs{
		legacyTx: tx,
	}, nil
}

type RlpPackable interface {
	packTxFields() ([]interface{}, error)
}

func toRlpUnsignedMsg(tx RlpPackable) ([]byte, error) {
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

func toRlpSignedMsg(tx RlpPackable, V, R, S big.Int) ([]byte, error) {
	packed1, err := tx.packTxFields()
	if err != nil {
		return nil, err
	}

	packed2, err := packSigFields(V, R, S)
	if err != nil {
		return nil, err
	}

	encoded, err := EncodeRLP(append(packed1, packed2...))
	if err != nil {
		return nil, fmt.Errorf("failed to encode rlp signed msg: %w", err)
	}
	return encoded, nil
}

func sender(tx EthTransaction) (address.Address, error) {
	msg, err := tx.ToRlpUnsignedMsg()
	if err != nil {
		return address.Undef, fmt.Errorf("failed to get rlp unsigned msg: %w", err)
	}

	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(msg)
	hash := hasher.Sum(nil)

	sig, err := tx.Signature()
	if err != nil {
		return address.Undef, fmt.Errorf("failed to get signature: %w", err)
	}

	sigData, err := tx.ToVerifiableSignature(sig.Data)
	if err != nil {
		return address.Undef, fmt.Errorf("failed to get verifiable signature: %w", err)
	}

	pubk, err := gocrypto.EcRecover(hash, sigData)
	if err != nil {
		return address.Undef, fmt.Errorf("failed to recover pubkey: %w", err)
	}

	ethAddr, err := EthAddressFromPubKey(pubk)
	if err != nil {
		return address.Undef, fmt.Errorf("failed to get eth address from pubkey: %w", err)
	}

	ea, err := CastEthAddress(ethAddr)
	if err != nil {
		return address.Undef, fmt.Errorf("failed to cast eth address: %w", err)
	}

	return ea.ToFilecoinAddress()
}
