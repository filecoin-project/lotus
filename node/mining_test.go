package node_test

import (
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/api/test"
)

func TestJoiningMiner(t *testing.T) {
	logging.SetLogLevel("miner", "ERROR")
	logging.SetLogLevel("chainstore", "ERROR")
	logging.SetLogLevel("chain", "ERROR")
	logging.SetLogLevel("sub", "ERROR")
	logging.SetLogLevel("storageminer", "ERROR")

	test.TestDealFlow(t, mockSbBuilder, 10*time.Millisecond, false)
}
