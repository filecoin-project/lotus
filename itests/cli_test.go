package itests

import (
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

	blockTime := 5 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)
	kit.RunClientTest(t, cli.Commands, client)
}
