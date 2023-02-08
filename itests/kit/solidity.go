package kit

import (
	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func EthTopicHash(sig string) ethtypes.EthHash {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(sig))
	var hash ethtypes.EthHash
	copy(hash[:], hasher.Sum(nil))
	return hash
}

func EthFunctionHash(sig string) []byte {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(sig))
	return hasher.Sum(nil)[:4]
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
