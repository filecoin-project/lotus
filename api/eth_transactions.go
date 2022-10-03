package api

import (
	"bytes"
	"encoding/binary"
	"fmt"
	mathbig "math/big"

	"golang.org/x/crypto/sha3"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	gocrypto "github.com/filecoin-project/go-crypto"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v8/evm"
	init8 "github.com/filecoin-project/go-state-types/builtin/v8/init"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

type EthTx struct {
	ChainID              EthUint64   `json:"chainId"`
	Nonce                EthUint64   `json:"nonce"`
	Hash                 EthHash     `json:"hash"`
	BlockHash            EthHash     `json:"blockHash"`
	BlockNumber          EthUint64   `json:"blockNumber"`
	TransactionIndex     EthUint64   `json:"transacionIndex"`
	From                 EthAddress  `json:"from"`
	To                   *EthAddress `json:"to"`
	Value                EthBigInt   `json:"value"`
	Type                 EthUint64   `json:"type"`
	Input                EthBytes    `json:"input"`
	Gas                  EthUint64   `json:"gas"`
	GasLimit             *EthUint64  `json:"gasLimit,omitempty"`
	MaxFeePerGas         EthBigInt   `json:"maxFeePerGas"`
	MaxPriorityFeePerGas EthBigInt   `json:"maxPriorityFeePerGas"`
	V                    EthBytes    `json:"v"`
	R                    EthBytes    `json:"r"`
	S                    EthBytes    `json:"s"`
}

type EthTxArgs struct {
	ChainID              int         `json:"chainId"`
	Nonce                int         `json:"nonce"`
	To                   *EthAddress `json:"to"`
	Value                big.Int     `json:"value"`
	MaxFeePerGas         big.Int     `json:"maxFeePerGas"`
	MaxPriorityFeePerGas big.Int     `json:"maxPrioritiyFeePerGas"`
	GasLimit             int         `json:"gasLimit"`
	Input                []byte      `json:"input"`
	V                    []byte      `json:"v"`
	R                    []byte      `json:"r"`
	S                    []byte      `json:"s"`
}

func NewEthTxArgsFromMessage(msg *types.Message) (EthTxArgs, error) {
	var to *EthAddress
	params := msg.Params
	if msg.To == builtintypes.InitActorAddr {
		to = nil

		var exec init8.ExecParams
		reader := bytes.NewReader(msg.Params)
		if err := exec.UnmarshalCBOR(reader); err != nil {
			return EthTxArgs{}, err
		}

		var evmParams evm.ConstructorParams
		reader1 := bytes.NewReader(exec.ConstructorParams)
		if err := evmParams.UnmarshalCBOR(reader1); err != nil {
			return EthTxArgs{}, err
		}
		params = evmParams.Bytecode
	} else {
		addr, err := EthAddressFromFilecoinIDAddress(msg.To)
		if err != nil {
			return EthTxArgs{}, nil
		}
		to = &addr
	}

	return EthTxArgs{
		ChainID:              build.Eip155ChainId,
		Nonce:                int(msg.Nonce),
		To:                   to,
		Value:                msg.Value,
		Input:                params,
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

	if tx.To == nil && tx.Input == nil {
		return nil, fmt.Errorf("to and input cannot both be empty")
	}

	if tx.To == nil {
		// this is a contract creation
		to = builtintypes.InitActorAddr

		constructorParams, err := actors.SerializeParams(&evm.ConstructorParams{
			Bytecode:  tx.Input,
			InputData: []byte{},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to serialize constructor params: %w", err)
		}

		evmActorCid, ok := actors.GetActorCodeID(actors.Version8, "evm")
		if !ok {
			return nil, fmt.Errorf("failed to lookup evm actor code CID")
		}

		params, err = actors.SerializeParams(&init8.ExecParams{
			CodeCID:           evmActorCid,
			ConstructorParams: constructorParams,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to serialize init actor exec params: %w", err)
		}
	} else {
		addr, err := tx.To.ToFilecoinAddress()
		if err != nil {
			return nil, err
		}
		to = addr
		params = tx.Input
	}

	msg := &types.Message{
		Nonce:      uint64(tx.Nonce),
		From:       from,
		To:         to,
		Value:      tx.Value,
		Method:     2,
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
	msg, err := tx.OriginalRlpMsg()
	if err != nil {
		return nil, err
	}

	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(msg)
	hash := hasher.Sum(nil)
	return hash, nil
}

func (tx *EthTxArgs) OriginalRlpMsg() ([]byte, error) {
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

	encoded, err := EncodeRLP(res)
	if err != nil {
		return nil, err
	}
	return append([]byte{0x02}, encoded...), nil
}

func (tx *EthTxArgs) Signature() (*typescrypto.Signature, error) {
	if tx.V == nil || tx.R == nil || tx.S == nil {
		return nil, fmt.Errorf("one of V, R, or S is nil")
	}
	sig := append([]byte{}, tx.R...)
	sig = append(sig, tx.S...)
	sig = append(sig, tx.V...)

	if len(sig) != 65 {
		return nil, fmt.Errorf("signature is not 65 bytes")
	}
	return &typescrypto.Signature{
		Type: typescrypto.SigTypeDelegated, Data: sig,
	}, nil
}

func (tx *EthTxArgs) Sender() (address.Address, error) {
	msg, err := tx.OriginalRlpMsg()
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

	return address.NewSecp256k1Address(pubk)
}

func parseEip1559Tx(data []byte) (*EthTxArgs, error) {
	if data[0] != 2 {
		return nil, xerrors.Errorf("not an EIP-1559 transaction: first byte is not 2")
	}

	d, err := DecodeRLP(data[1:])
	if err != nil {
		return nil, err
	}
	decoded, ok := d.([]interface{})
	if !ok {
		return nil, xerrors.Errorf("not an EIP-1559 transaction: decoded data is not a list")
	}

	if len(decoded) != 9 && len(decoded) != 12 {
		return nil, xerrors.Errorf("not an EIP-1559 transaction: should have 6 or 9 elements in the list")
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

	V, err := parseBytes(decoded[9])
	if err != nil {
		return nil, err
	}

	if len(V) == 0 {
		V = []byte{0}
	}

	R, err := parseBytes(decoded[10])
	if err != nil {
		return nil, err
	}

	S, err := parseBytes(decoded[11])
	if err != nil {
		return nil, err
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
		R:                    padLeadingZeros(R, 32),
		S:                    padLeadingZeros(S, 32),
		V:                    V,
	}
	return &args, nil
}

func ParseEthTxArgs(data []byte) (*EthTxArgs, error) {
	if data[0] > 0x7f {
		// legacy transaction
		return nil, xerrors.Errorf("legacy transaction is not supported")
	} else if data[0] == 1 {
		// EIP-2930
		return nil, xerrors.Errorf("EIP-2930 transaction is not supported")
	} else if data[0] == 2 {
		// EIP-1559
		return parseEip1559Tx(data)
	}
	return nil, xerrors.Errorf("unsupported transaction type")
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
		return 0, xerrors.Errorf("cannot parse interface to int: input is not a byte array")
	}
	if len(data) == 0 {
		return 0, nil
	}
	if len(data) > 8 {
		return 0, xerrors.Errorf("cannot parse interface to int: length is more than 8 bytes")
	}
	var value int64
	r := bytes.NewReader(append(make([]byte, 8-len(data)), data...))
	if err := binary.Read(r, binary.BigEndian, &value); err != nil {
		return 0, xerrors.Errorf("cannot parse interface to EthUint64: %w", err)
	}
	return int(value), nil
}

func parseBigInt(v interface{}) (big.Int, error) {
	data, ok := v.([]byte)
	if !ok {
		return big.Zero(), xerrors.Errorf("cannot parse interface to big.Int: input is not a byte array")
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
		return nil, xerrors.Errorf("cannot parse interface into bytes: input is not a byte array")
	}
	return val, nil
}

func parseEthAddr(v interface{}) (*EthAddress, error) {
	b, err := parseBytes(v)
	if err != nil {
		return nil, err
	}
	if b == nil || len(b) == 0 {
		return nil, nil
	}
	addr, err := EthAddressFromBytes(b)
	if err != nil {
		return nil, err
	}
	return &addr, nil
}
