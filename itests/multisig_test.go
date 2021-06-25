package itests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/itests/multisig"
)

// TestMultisig does a basic test to exercise the multisig CLI commands
func TestMultisig(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	kit.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	clientNode, _ := kit.StartOneNodeOneMiner(ctx, t, blocktime)

	multisig.RunMultisigTests(t, clientNode)
}
