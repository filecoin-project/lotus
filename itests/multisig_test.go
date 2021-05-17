package itests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/cli"
)

// TestMultisig does a basic test to exercise the multisig CLI
// commands
func TestMultisig(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	clientNode, _ := StartOneNodeOneMiner(ctx, t, blocktime)
	RunMultisigTest(t, cli.Commands, clientNode)
}
