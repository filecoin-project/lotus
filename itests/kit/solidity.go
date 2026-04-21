package kit

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-keccak"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func EthTopicHash(sig string) ethtypes.EthHash {
	hasher := keccak.NewLegacyKeccak256()
	hasher.Write([]byte(sig))
	var hash ethtypes.EthHash
	copy(hash[:], hasher.Sum(nil))
	return hash
}

func EthFunctionHash(sig string) []byte {
	hasher := keccak.NewLegacyKeccak256()
	hasher.Write([]byte(sig))
	return hasher.Sum(nil)[:4]
}

// EvmWordBytes right-aligns input into a 32-byte big-endian EVM word.
// Panics if input is longer than 32 bytes.
func EvmWordBytes(input []byte) []byte {
	if len(input) > 32 {
		panic(fmt.Sprintf("EvmWordBytes: input of %d bytes exceeds word size", len(input)))
	}
	word := make([]byte, 32)
	copy(word[32-len(input):], input)
	return word
}

// EvmWordUint64 encodes n as a 32-byte big-endian EVM word.
func EvmWordUint64(n uint64) []byte {
	word := make([]byte, 32)
	binary.BigEndian.PutUint64(word[24:], n)
	return word
}

// EvmWordFromAddr resolves a Filecoin address to its corresponding Ethereum
// address and returns it as a 32-byte big-endian EVM word.
func EvmWordFromAddr(ctx context.Context, t *testing.T, client *TestFullNode, from address.Address) []byte {
	t.Helper()
	fromID, err := client.StateLookupID(ctx, from, types.EmptyTSK)
	require.NoError(t, err)
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(fromID)
	require.NoError(t, err)
	return EvmWordBytes(ethAddr[:])
}

// EvmDecodeUint64 interprets the low 8 bytes of an EVM word as a big-endian
// uint64. Returns an error if output is shorter than 8 bytes.
func EvmDecodeUint64(output []byte) (uint64, error) {
	if len(output) < 8 {
		return 0, fmt.Errorf("EvmDecodeUint64: output too short: %d bytes, need 8", len(output))
	}
	var n uint64
	if err := binary.Read(bytes.NewReader(output[len(output)-8:]), binary.BigEndian, &n); err != nil {
		return 0, err
	}
	return n, nil
}

// SolidityContractDef holds information about one of the test contracts
type SolidityContractDef struct {
	Filename string                      // filename of the hex of the contract, e.g. contracts/EventMatrix.hex
	Fn       map[string][]byte           // mapping of function names to 32-bit selector
	Ev       map[string]ethtypes.EthHash // mapping of event names to 256-bit signature hashes
}

var EventMatrixContract = SolidityContractDef{
	Filename: "contracts/EventMatrix.hex",
	Fn: map[string][]byte{
		"logEventZeroData":             EthFunctionHash("logEventZeroData()"),
		"logEventOneData":              EthFunctionHash("logEventOneData(uint256)"),
		"logEventTwoData":              EthFunctionHash("logEventTwoData(uint256,uint256)"),
		"logEventThreeData":            EthFunctionHash("logEventThreeData(uint256,uint256,uint256)"),
		"logEventFourData":             EthFunctionHash("logEventFourData(uint256,uint256,uint256,uint256)"),
		"logEventOneIndexed":           EthFunctionHash("logEventOneIndexed(uint256)"),
		"logEventTwoIndexed":           EthFunctionHash("logEventTwoIndexed(uint256,uint256)"),
		"logEventThreeIndexed":         EthFunctionHash("logEventThreeIndexed(uint256,uint256,uint256)"),
		"logEventOneIndexedWithData":   EthFunctionHash("logEventOneIndexedWithData(uint256,uint256)"),
		"logEventTwoIndexedWithData":   EthFunctionHash("logEventTwoIndexedWithData(uint256,uint256,uint256)"),
		"logEventThreeIndexedWithData": EthFunctionHash("logEventThreeIndexedWithData(uint256,uint256,uint256,uint256)"),
	},
	Ev: map[string]ethtypes.EthHash{
		"EventZeroData":             EthTopicHash("EventZeroData()"),
		"EventOneData":              EthTopicHash("EventOneData(uint256)"),
		"EventTwoData":              EthTopicHash("EventTwoData(uint256,uint256)"),
		"EventThreeData":            EthTopicHash("EventThreeData(uint256,uint256,uint256)"),
		"EventFourData":             EthTopicHash("EventFourData(uint256,uint256,uint256,uint256)"),
		"EventOneIndexed":           EthTopicHash("EventOneIndexed(uint256)"),
		"EventTwoIndexed":           EthTopicHash("EventTwoIndexed(uint256,uint256)"),
		"EventThreeIndexed":         EthTopicHash("EventThreeIndexed(uint256,uint256,uint256)"),
		"EventOneIndexedWithData":   EthTopicHash("EventOneIndexedWithData(uint256,uint256)"),
		"EventTwoIndexedWithData":   EthTopicHash("EventTwoIndexedWithData(uint256,uint256,uint256)"),
		"EventThreeIndexedWithData": EthTopicHash("EventThreeIndexedWithData(uint256,uint256,uint256,uint256)"),
	},
}

var EventsContract = SolidityContractDef{
	Filename: "contracts/events.bin",
	Fn: map[string][]byte{
		"log_zero_data":   {0x00, 0x00, 0x00, 0x00},
		"log_zero_nodata": {0x00, 0x00, 0x00, 0x01},
		"log_four_data":   {0x00, 0x00, 0x00, 0x02},
	},
	Ev: map[string]ethtypes.EthHash{},
}
