package api

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mathbig "math/big"
	"strconv"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/minio/blake2b-simd"
	"github.com/multiformats/go-multihash"
	"github.com/multiformats/go-varint"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"

	"github.com/filecoin-project/lotus/build"
)

type EthUint64 uint64

func (e EthUint64) MarshalJSON() ([]byte, error) {
	if e == 0 {
		return json.Marshal("0x0")
	}
	return json.Marshal(fmt.Sprintf("0x%x", e))
}

func (e *EthUint64) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsedInt, err := strconv.ParseUint(strings.Replace(s, "0x", "", -1), 16, 64)
	if err != nil {
		return err
	}
	eint := EthUint64(parsedInt)
	*e = eint
	return nil
}

type EthBigInt big.Int

var EthBigIntZero = EthBigInt{Int: big.Zero().Int}

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

type EthBytes []byte

func (e EthBytes) MarshalJSON() ([]byte, error) {
	if len(e) == 0 {
		return json.Marshal("0x00")
	}
	s := hex.EncodeToString(e)
	if len(s)%2 == 1 {
		s = "0" + s
	}
	return json.Marshal("0x" + s)
}

func (e *EthBytes) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	s = strings.Replace(s, "0x", "", -1)
	if len(s)%2 == 1 {
		s = "0" + s
	}

	decoded, err := hex.DecodeString(s)
	if err != nil {
		return err
	}

	*e = decoded
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
	Difficulty    EthUint64 `json:"difficulty"`
	Number        EthUint64 `json:"number"`
	GasLimit      EthUint64 `json:"gasLimit"`
	GasUsed       EthUint64 `json:"gasUsed"`
	Timestamp     EthUint64 `json:"timestamp"`
	Extradata     []byte    `json:"extraData"`
	MixHash       EthHash   `json:"mixHash"`
	Nonce         EthNonce  `json:"nonce"`
	BaseFeePerGas EthBigInt `json:"baseFeePerGas"`
	Size          EthUint64 `json:"size"`
	// can be []EthTx or []string depending on query params
	Transactions []interface{} `json:"transactions"`
	Uncles       []EthHash     `json:"uncles"`
}

var (
	EmptyEthHash  = EthHash{}
	EmptyEthInt   = EthUint64(0)
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
		GasLimit:         EthUint64(build.BlockGasLimit), // TODO we map Ethereum blocks to Filecoin tipsets; this is inconsistent.
		Uncles:           []EthHash{},
		Transactions:     []interface{}{},
	}
}

type EthCall struct {
	From     *EthAddress `json:"from"`
	To       *EthAddress `json:"to"`
	Gas      EthUint64   `json:"gas"`
	GasPrice EthBigInt   `json:"gasPrice"`
	Value    EthBigInt   `json:"value"`
	Data     EthBytes    `json:"data"`
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
	TransactionHash  EthHash     `json:"transactionHash"`
	TransactionIndex EthUint64   `json:"transactionIndex"`
	BlockHash        EthHash     `json:"blockHash"`
	BlockNumber      EthUint64   `json:"blockNumber"`
	From             EthAddress  `json:"from"`
	To               *EthAddress `json:"to"`
	// Logs
	// LogsBloom
	StateRoot         EthHash     `json:"root"`
	Status            EthUint64   `json:"status"`
	ContractAddress   *EthAddress `json:"contractAddress"`
	CumulativeGasUsed EthUint64   `json:"cumulativeGasUsed"`
	GasUsed           EthUint64   `json:"gasUsed"`
	EffectiveGasPrice EthBigInt   `json:"effectiveGasPrice"`
	LogsBloom         EthBytes    `json:"logsBloom"`
	Logs              []string    `json:"logs"`
}

func NewEthTxReceipt(tx EthTx, lookup *MsgLookup, replay *InvocResult) (EthTxReceipt, error) {
	receipt := EthTxReceipt{
		TransactionHash:  tx.Hash,
		TransactionIndex: tx.TransactionIndex,
		BlockHash:        tx.BlockHash,
		BlockNumber:      tx.BlockNumber,
		From:             tx.From,
		To:               tx.To,
		StateRoot:        EmptyEthHash,
		LogsBloom:        []byte{0},
		Logs:             []string{},
	}

	if receipt.To == nil && lookup.Receipt.ExitCode.IsSuccess() {
		// Create and Create2 return the same things.
		var ret eam.CreateReturn
		if err := ret.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)); err != nil {
			return EthTxReceipt{}, xerrors.Errorf("failed to parse contract creation result: %w", err)
		}
		addr := EthAddress(ret.EthAddress)
		receipt.ContractAddress = &addr
	}

	if lookup.Receipt.ExitCode.IsSuccess() {
		receipt.Status = 1
	}
	if lookup.Receipt.ExitCode.IsError() {
		receipt.Status = 0
	}

	receipt.GasUsed = EthUint64(lookup.Receipt.GasUsed)

	// TODO: handle CumulativeGasUsed
	receipt.CumulativeGasUsed = EmptyEthInt

	effectiveGasPrice := big.Div(replay.GasCost.TotalCost, big.NewInt(lookup.Receipt.GasUsed))
	receipt.EffectiveGasPrice = EthBigInt(effectiveGasPrice)
	return receipt, nil
}

const (
	EthAddressLength = 20
	EthHashLength    = 32
)

type EthNonce [8]byte

func (n EthNonce) String() string {
	return "0x" + hex.EncodeToString(n[:])
}

func (n EthNonce) MarshalJSON() ([]byte, error) {
	return json.Marshal(n.String())
}

type EthAddress [EthAddressLength]byte

func (ea EthAddress) String() string {
	return "0x" + hex.EncodeToString(ea[:])
}

func (ea EthAddress) MarshalJSON() ([]byte, error) {
	return json.Marshal(ea.String())
}

func (ea *EthAddress) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	addr, err := EthAddressFromHex(s)
	if err != nil {
		return err
	}
	copy(ea[:], addr[:])
	return nil
}

func (ea EthAddress) ToFilecoinAddress() (address.Address, error) {
	idmask := [12]byte{0xff}
	if bytes.Equal(ea[:12], idmask[:]) {
		// This is a masked ID address.
		id := binary.BigEndian.Uint64(ea[12:])
		return address.NewIDAddress(id)
	}

	// Otherwise, translate the address into an address controlled by the
	// Ethereum Address Manager.
	addr, err := address.NewDelegatedAddress(builtin.EthereumAddressManagerActorID, ea[:])
	if err != nil {
		return address.Undef, fmt.Errorf("failed to translate supplied address (%s) into a "+
			"Filecoin f4 address: %w", hex.EncodeToString(ea[:]), err)
	}
	return addr, nil
}

func TryEthAddressFromFilecoinAddress(addr address.Address, allowId bool) (EthAddress, bool, error) {
	switch addr.Protocol() {
	case address.ID:
		if !allowId {
			return EthAddress{}, false, nil
		}
		id, err := address.IDFromAddress(addr)
		if err != nil {
			return EthAddress{}, false, err
		}
		var ethaddr EthAddress
		ethaddr[0] = 0xff
		binary.BigEndian.PutUint64(ethaddr[12:], id)
		return ethaddr, true, nil
	case address.Delegated:
		payload := addr.Payload()
		namespace, n, err := varint.FromUvarint(payload)
		if err != nil {
			return EthAddress{}, false, xerrors.Errorf("invalid delegated address namespace in: %s", addr)
		}
		payload = payload[n:]
		if namespace == builtin.EthereumAddressManagerActorID {
			addr, err := EthAddressFromBytes(payload)
			return addr, err == nil, err
		}
	}
	return EthAddress{}, false, nil
}

func EthAddressFromFilecoinAddress(addr address.Address) (EthAddress, error) {
	ethAddr, ok, err := TryEthAddressFromFilecoinAddress(addr, true)
	if !ok && err == nil {
		err = xerrors.Errorf("failed to convert filecoin address %s to an equivalent eth address", addr)
	}
	return ethAddr, err
}

func EthAddressFromHex(s string) (EthAddress, error) {
	handlePrefix(&s)
	b, err := decodeHexString(s, EthAddressLength)
	if err != nil {
		return EthAddress{}, err
	}
	var h EthAddress
	copy(h[EthAddressLength-len(b):], b)
	return h, nil
}

func EthAddressFromBytes(b []byte) (EthAddress, error) {
	var a EthAddress
	if len(b) != EthAddressLength {
		return EthAddress{}, xerrors.Errorf("cannot parse bytes into anœ EthAddress: incorrect input length")
	}
	copy(a[:], b[:])
	return a, nil
}

type EthHash [EthHashLength]byte

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
	b, err := decodeHexString(s, EthHashLength)
	if err != nil {
		return EthHash{}, err
	}
	var h EthHash
	copy(h[EthHashLength-len(b):], b)
	return h, nil
}

func EthHashData(b []byte) EthHash {
	return EthHash(blake2b.Sum256(b))
}

func (h EthHash) String() string {
	return "0x" + hex.EncodeToString(h[:])
}

func (h EthHash) ToCid() cid.Cid {
	// err is always nil
	mh, _ := multihash.EncodeName(h[:], "blake2b-256")

	return cid.NewCidV1(cid.DagCBOR, mh)
}

type EthFeeHistory struct {
	OldestBlock   uint64         `json:"oldestBlock"`
	BaseFeePerGas []EthBigInt    `json:"baseFeePerGas"`
	GasUsedRatio  []float64      `json:"gasUsedRatio"`
	Reward        *[][]EthBigInt `json:"reward,omitempty"`
}

// An opaque identifier generated by the Lotus node to refer to an installed filter.
type EthFilterID string

// An opaque identifier generated by the Lotus node to refer to an active subscription.
type EthSubscriptionID string

type EthFilterSpec struct {
	// Interpreted as an epoch or one of "latest" for last mined block, "earliest" for first,
	// "pending" for not yet committed messages.
	// Optional, default: "latest".
	FromBlock *string `json:"fromBlock,omitempty"`

	// Interpreted as an epoch or one of "latest" for last mined block, "earliest" for first,
	// "pending" for not yet committed messages.
	// Optional, default: "latest".
	ToBlock *string `json:"toBlock,omitempty"`

	// Actor address or a list of addresses from which event logs should originate.
	// Optional, default nil.
	// The JSON decoding must treat a string as equivalent to an array with one value, for example
	// "0x8888f1f195afa192cfee86069858" must be decoded as [ "0x8888f1f195afa192cfee86069858" ]
	Address EthAddressList `json:"address"`

	// List of topics to be matched.
	// Optional, default: empty list
	Topics EthTopicSpec `json:"topics"`

	// Restricts event logs returned to those in receipts contained in the tipset this block is part of.
	// If BlockHash is present in in the filter criteria, then neither FromBlock nor ToBlock are allowed.
	// Added in EIP-234
	BlockHash *EthHash `json:"blockHash,omitempty"`
}

// EthAddressSpec represents a list of addresses.
// The JSON decoding must treat a string as equivalent to an array with one value, for example
// "0x8888f1f195afa192cfee86069858" must be decoded as [ "0x8888f1f195afa192cfee86069858" ]
type EthAddressList []EthAddress

func (e *EthAddressList) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		var addrs []EthAddress
		err := json.Unmarshal(b, &addrs)
		if err != nil {
			return err
		}
		*e = addrs
		return nil
	}
	var addr EthAddress
	err := json.Unmarshal(b, &addr)
	if err != nil {
		return err
	}
	*e = []EthAddress{addr}
	return nil
}

// TopicSpec represents a specification for matching by topic. An empty spec means all topics
// will be matched. Otherwise topics are matched conjunctively in the first dimension of the
// slice and disjunctively in the second dimension. Topics are matched in order.
// An event log with topics [A, B] will be matched by the following topic specs:
// [] "all"
// [[A]] "A in first position (and anything after)"
// [[A]] "A in first position (and anything after)"
// [nil, [B] ] "anything in first position AND B in second position (and anything after)"
// [[A], [B]] "A in first position AND B in second position (and anything after)"
// [[A, B], [A, B]] "(A OR B) in first position AND (A OR B) in second position (and anything after)"
//
// The JSON decoding must treat string values as equivalent to arrays with one value, for example
// { "A", [ "B", "C" ] } must be decoded as [ [ A ], [ B, C ] ]
type EthTopicSpec []EthHashList

type EthHashList []EthHash

func (e *EthHashList) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte{'n', 'u', 'l', 'l'}) {
		return nil
	}
	if len(b) > 0 && b[0] == '[' {
		var hashes []EthHash
		err := json.Unmarshal(b, &hashes)
		if err != nil {
			return err
		}
		*e = hashes
		return nil
	}
	var hash EthHash
	err := json.Unmarshal(b, &hash)
	if err != nil {
		return err
	}
	*e = []EthHash{hash}
	return nil
}

// FilterResult represents the response from executing a filter: a list of bloack hashes, a list of transaction hashes
// or a list of logs
// This is a union type. Only one field will be populated.
// The JSON encoding must produce an array of the populated field.
type EthFilterResult struct {
	// List of block hashes. Only populated when the filter has been installed via EthNewBlockFilter
	NewBlockHashes []EthHash

	// List of transaction hashes. Only populated when the filter has been installed via EthNewPendingTransactionFilter
	NewTransactionHashes []EthHash

	// List of event logs. Only populated when the filter has been installed via EthNewFilter
	NewLogs []EthLog
}

func (h EthFilterResult) MarshalJSON() ([]byte, error) {
	if h.NewBlockHashes != nil {
		return json.Marshal(h.NewBlockHashes)
	}
	if h.NewTransactionHashes != nil {
		return json.Marshal(h.NewTransactionHashes)
	}
	if h.NewLogs != nil {
		return json.Marshal(h.NewLogs)
	}
	return []byte{'[', ']'}, nil
}

type EthLog struct {
	// Address is the address of the actor that produced the event log.
	Address EthAddress `json:"address"`

	// Data is the values of the event log, excluding topics
	Data []EthHash `json:"data"`

	// List of topics associated with the event log.
	Topics []EthHash `json:"topics"`

	// Following fields are derived from the transaction containing the log

	// Indicates whether the log was removed due to a chain reorganization.
	Removed bool `json:"removed"`

	// LogIndex is the index of the event log in the sequence of events produced by the message execution.
	// (this is the index in the events AMT on the message receipt)
	LogIndex EthUint64 `json:"logIndex"`

	// TransactionIndex is the index in the tipset of the transaction that produced the event log.
	// The index corresponds to the sequence of messages produced by ChainGetParentMessages
	TransactionIndex EthUint64 `json:"transactionIndex"`

	// TransactionHash is the cid of the transaction that produced the event log.
	TransactionHash EthHash `json:"transactionHash"`

	// BlockHash is the hash of a block in the tipset containing the message receipt of the message execution.
	// This may be passed to ChainGetParentReceipts to obtain a list of receipts. The receipt
	// containing the events will be at TransactionIndex in the receipt list.
	BlockHash EthHash `json:"blockHash"`

	// BlockNumber is the epoch at which the message was executed. This is the epoch containing
	// the message receipt.
	BlockNumber EthUint64 `json:"blockNumber"`
}

type EthSubscriptionParams struct {
	// List of topics to be matched.
	// Optional, default: empty list
	Topics EthTopicSpec `json:"topics,omitempty"`
}

type EthSubscriptionResponse struct {
	// The persistent identifier for the subscription which can be used to unsubscribe.
	SubscriptionID EthSubscriptionID `json:"subscription"`

	// The object matching the subscription. This may be a Block (tipset), a Transaction (message) or an EthLog
	Result interface{} `json:"result"`
}
