package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	xerrors "golang.org/x/xerrors"
)

type EthInt int64

func (e EthInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("0x%x", e))
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
}

type EthTx struct {
}

type EthTxReceipt struct {
}

type EthAddress [20]byte

func (a EthAddress) String() string {
	return "0x" + hex.EncodeToString(a[:])
}

func (a EthAddress) MarshalJSON() ([]byte, error) {
	return json.Marshal((a.String()))
}

type EthHash [32]byte

func (h EthHash) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

type EthNonce [8]byte

func (n EthNonce) String() string {
	return "0x" + hex.EncodeToString(n[:])
}

func (n EthNonce) MarshalJSON() ([]byte, error) {
	return json.Marshal((n.String()))
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
