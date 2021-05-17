package itests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/cli"
)

// TestClient does a basic test to exercise the client CLI commands.
func TestClient(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	clientNode, _ := StartOneNodeOneMiner(ctx, t, blocktime)
	RunClientTest(t, cli.Commands, clientNode)
}
