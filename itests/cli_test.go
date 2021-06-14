package itests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestClient does a basic test to exercise the client CLI commands.
func TestClient(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	kit.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	clientNode, _ := kit.StartOneNodeOneMiner(ctx, t, blocktime)
	kit.RunClientTest(t, cli.Commands, clientNode)
}
