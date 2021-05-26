package cli

import (
	"context"
	"os"
	"testing"
	"time"

	clitest "github.com/filecoin-project/lotus/cli/test"
	logging "github.com/ipfs/go-log/v2"
)

// TestClient does a basic test to exercise the client CLI
// commands
func TestClient(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	logging.SetLogLevel("markets", "debug")
	logging.SetLogLevel("dt-impl", "debug")
	logging.SetLogLevel("dt-chanmon", "debug")
	logging.SetLogLevel(" dt_graphsync", "debug")

	clitest.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	clientNode, _ := clitest.StartOneNodeOneMiner(ctx, t, blocktime)
	clitest.RunClientTest(t, Commands, clientNode)
}
