package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	xerrors "golang.org/x/xerrors"
)

type EthInt int64

func (e EthInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("0x%x", e))
}

type EthBigInt big.Int

func (e EthBigInt) MarshalJSON() ([]byte, error) {
	if e.Int == nil {
		return json.Marshal("0x0")
	}
	return json.Marshal(fmt.Sprintf("0x%x", e.Int))
}

type EthBlock struct {
	ParentHash       EthHash    `json:"parentHash"`
	Sha3Uncles       EthHash    `json:"sha3Uncles"`
	Miner            EthAddress `json:"miner"`
	StateRoot        EthHash    `json:"stateRoot"`
	TransactionsRoot EthHash    `json:"transactionsRoot"`
	ReceiptsRoot     EthHash    `json:"receiptsRoot"`
	// TODO: include LogsBloom
	Difficulty    EthInt   `json:"difficulty"`
	Number        EthInt   `json:"number"`
	GasLimit      EthInt   `json:"gasLimit"`
	GasUsed       EthInt   `json:"gasUsed"`
	Timestamp     EthInt   `json:"timestamp"`
	Extradata     []byte   `json:"extraData"`
	MixHash       EthHash  `json:"mixHash"`
	Nonce         EthNonce `json:"nonce"`
	BaseFeePerGas EthInt   `json:"baseFeePerGas"`
	Transactions  EthTx    `json:"transactions"`
}

type EthTx struct {
	ChainID              *EthInt    `json:"chainId"`
	Nonce                uint64     `json:"nonce"`
	Hash                 EthHash    `json:"hash"`
	BlockHash            EthHash    `json:"blockHash"`
	BlockNumber          EthHash    `json:"blockNumber"`
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

type EthAddress [20]byte

func (a EthAddress) String() string {
	return "0x" + hex.EncodeToString(a[:])
}

func (a EthAddress) MarshalJSON() ([]byte, error) {
	return json.Marshal((a.String()))
}

type EthNonce [8]byte

func (n EthNonce) String() string {
	return "0x" + hex.EncodeToString(n[:])
}

func (n EthNonce) MarshalJSON() ([]byte, error) {
	return json.Marshal((n.String()))
}

type EthHash [32]byte

func (h EthHash) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

func fromHexString(s string) (EthHash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return EthHash{}, xerrors.Errorf("cannot parse cid hash: %w", err)
	}

	if len(b) > 32 {
		return EthHash{}, xerrors.Errorf("length of decoded bytes is longer than 32")
	}

	var h EthHash
	copy(h[32-len(b):], b)

	return h, nil
}

func EthHashFromCid(c cid.Cid) (EthHash, error) {
	return fromHexString(c.Hash().HexString()[8:])
}

func EthHashFromHex(s string) (EthHash, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if len(s)%2 == 1 {
		s = "0" + s
	}
	return fromHexString(s)
}

func (h EthHash) String() string {
	return "0x" + hex.EncodeToString(h[:])
}

func (h EthHash) ToCid() cid.Cid {
	// err is always nil
	mh, _ := multihash.EncodeName(h[:], "blake2b-256")

	return cid.NewCidV1(cid.DagCBOR, mh)
}
