package api

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mathbig "math/big"
	"strconv"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build"
)

type EthInt int64

func (e EthInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("0x%x", e))
}

func (e *EthInt) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsedInt, err := strconv.ParseInt(strings.Replace(s, "0x", "", -1), 16, 64)
	if err != nil {
		return err
	}
	eint := EthInt(parsedInt)
	*e = eint
	return nil
}

type EthBigInt big.Int

var (
	EthBigIntZero = EthBigInt{Int: big.Zero().Int}
)

func (e EthBigInt) MarshalJSON() ([]byte, error) {
	if e.Int == nil {
		return json.Marshal("0x0")
	}
	return json.Marshal(fmt.Sprintf("0x%x", e.Int))
}

func (e *EthBigInt) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	replaced := strings.Replace(s, "0x", "", -1)
	if len(replaced)%2 == 1 {
		replaced = "0" + replaced
	}

	i := new(mathbig.Int)
	i.SetString(replaced, 16)

	*e = EthBigInt(big.NewFromGo(i))
	return nil
}

type EthBlock struct {
	ParentHash       EthHash    `json:"parentHash"`
	Sha3Uncles       EthHash    `json:"sha3Uncles"`
	Miner            EthAddress `json:"miner"`
	StateRoot        EthHash    `json:"stateRoot"`
	TransactionsRoot EthHash    `json:"transactionsRoot"`
	ReceiptsRoot     EthHash    `json:"receiptsRoot"`
	// TODO: include LogsBloom
	Difficulty    EthInt    `json:"difficulty"`
	Number        EthInt    `json:"number"`
	GasLimit      EthInt    `json:"gasLimit"`
	GasUsed       EthInt    `json:"gasUsed"`
	Timestamp     EthInt    `json:"timestamp"`
	Extradata     []byte    `json:"extraData"`
	MixHash       EthHash   `json:"mixHash"`
	Nonce         EthNonce  `json:"nonce"`
	BaseFeePerGas EthBigInt `json:"baseFeePerGas"`
	Size          EthInt    `json:"size"`
	// can be []EthTx or []string depending on query params
	Transactions []interface{} `json:"transactions"`
	Uncles       []EthHash     `json:"uncles"`
}

var (
	EmptyEthHash  = EthHash{}
	EmptyEthInt   = EthInt(0)
	EmptyEthNonce = [8]byte{0, 0, 0, 0, 0, 0, 0, 0}
)

func NewEthBlock() EthBlock {
	return EthBlock{
		Sha3Uncles:       EmptyEthHash,
		StateRoot:        EmptyEthHash,
		TransactionsRoot: EmptyEthHash,
		ReceiptsRoot:     EmptyEthHash,
		Difficulty:       EmptyEthInt,
		Extradata:        []byte{},
		MixHash:          EmptyEthHash,
		Nonce:            EmptyEthNonce,
		GasLimit:         EthInt(build.BlockGasLimit), // TODO we map Ethereum blocks to Filecoin tipsets; this is inconsistent.
		Uncles:           []EthHash{},
		Transactions:     []interface{}{},
	}
}

type EthTx struct {
	ChainID              EthInt     `json:"chainId"`
	Nonce                uint64     `json:"nonce"`
	Hash                 EthHash    `json:"hash"`
	BlockHash            EthHash    `json:"blockHash"`
	BlockNumber          EthInt     `json:"blockNumber"`
	TransactionIndex     EthInt     `json:"transacionIndex"`
	From                 EthAddress `json:"from"`
	To                   EthAddress `json:"to"`
	Value                EthBigInt  `json:"value"`
	Type                 EthInt     `json:"type"`
	Input                []byte     `json:"input"`
	Gas                  EthInt     `json:"gas"`
	MaxFeePerGas         EthBigInt  `json:"maxFeePerGas"`
	MaxPriorityFeePerGas EthBigInt  `json:"maxPriorityFeePerGas"`
	V                    EthBigInt  `json:"v"`
	R                    EthBigInt  `json:"r"`
	S                    EthBigInt  `json:"s"`
}

type EthCall struct {
	From     EthAddress `json:"from"`
	To       EthAddress `json:"to"`
	Gas      EthInt     `json:"gas"`
	GasPrice EthBigInt  `json:"gasPrice"`
	Value    EthBigInt  `json:"value"`
	Data     []byte     `json:"data"`
}

func (c *EthCall) UnmarshalJSON(b []byte) error {
	type TempEthCall EthCall
	var params TempEthCall

	if err := json.Unmarshal(b, &params); err != nil {
		return err
	}
	*c = EthCall(params)
	return nil
}

type EthTxReceipt struct {
	TransactionHash  EthHash    `json:"transactionHash"`
	TransactionIndex EthInt     `json:"transacionIndex"`
	BlockHash        EthHash    `json:"blockHash"`
	BlockNumber      EthHash    `json:"blockNumber"`
	From             EthAddress `json:"from"`
	To               EthAddress `json:"to"`
	// Logs
	// LogsBloom
	StateRoot         EthHash     `json:"root"`
	Status            EthInt      `json:"status"`
	ContractAddress   *EthAddress `json:"contractAddress"`
	CumulativeGasUsed EthBigInt   `json:"cumulativeGasUsed"`
	GasUsed           EthBigInt   `json:"gasUsed"`
	EffectiveGasPrice EthBigInt   `json:"effectiveGasPrice"`
}

const (
	ETH_ADDRESS_LENGTH = 20
	ETH_HASH_LENGTH    = 32
)

type EthNonce [8]byte

func (n EthNonce) String() string {
	return "0x" + hex.EncodeToString(n[:])
}

func (n EthNonce) MarshalJSON() ([]byte, error) {
	return json.Marshal((n.String()))
}

type EthAddress [ETH_ADDRESS_LENGTH]byte

func (a EthAddress) String() string {
	return "0x" + hex.EncodeToString(a[:])
}

func (a EthAddress) MarshalJSON() ([]byte, error) {
	return json.Marshal((a.String()))
}

func (a *EthAddress) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	addr, err := EthAddressFromHex(s)
	if err != nil {
		return err
	}
	copy(a[:], addr[:])
	return nil
}

func (a EthAddress) ToFilecoinAddress() (address.Address, error) {
	id := binary.BigEndian.Uint64(a[12:])
	return address.NewIDAddress(id)
}

func EthAddressFromFilecoinIDAddress(addr address.Address) (EthAddress, error) {
	id, err := address.IDFromAddress(addr)
	if err != nil {
		return EthAddress{}, err
	}
	buf := make([]byte, ETH_ADDRESS_LENGTH)
	buf[0] = 0xff
	binary.BigEndian.PutUint64(buf[12:], id)

	var ethaddr EthAddress
	copy(ethaddr[:], buf)
	return ethaddr, nil
}

func EthAddressFromHex(s string) (EthAddress, error) {
	handlePrefix(&s)
	b, err := decodeHexString(s, ETH_ADDRESS_LENGTH)
	if err != nil {
		return EthAddress{}, err
	}
	var h EthAddress
	copy(h[ETH_ADDRESS_LENGTH-len(b):], b)
	return h, nil
}

type EthHash [ETH_HASH_LENGTH]byte

func (h EthHash) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

func (h *EthHash) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	hash, err := EthHashFromHex(s)
	if err != nil {
		return err
	}
	copy(h[:], hash[:])
	return nil
}

func handlePrefix(s *string) {
	if strings.HasPrefix(*s, "0x") || strings.HasPrefix(*s, "0X") {
		*s = (*s)[2:]
	}
	if len(*s)%2 == 1 {
		*s = "0" + *s
	}
}

func decodeHexString(s string, length int) ([]byte, error) {
	b, err := hex.DecodeString(s)

	if err != nil {
		return []byte{}, xerrors.Errorf("cannot parse hash: %w", err)
	}

	if len(b) > length {
		return []byte{}, xerrors.Errorf("length of decoded bytes is longer than %d", length)
	}

	return b, nil
}

func EthHashFromCid(c cid.Cid) (EthHash, error) {
	return EthHashFromHex(c.Hash().HexString()[8:])
}

func EthHashFromHex(s string) (EthHash, error) {
	handlePrefix(&s)
	b, err := decodeHexString(s, ETH_HASH_LENGTH)
	if err != nil {
		return EthHash{}, err
	}
	var h EthHash
	copy(h[ETH_HASH_LENGTH-len(b):], b)
	return h, nil
}

func (h EthHash) String() string {
	return "0x" + hex.EncodeToString(h[:])
}

func (h EthHash) ToCid() cid.Cid {
	// err is always nil
	mh, _ := multihash.EncodeName(h[:], "blake2b-256")

	return cid.NewCidV1(cid.DagCBOR, mh)
}
