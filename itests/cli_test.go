package itests

import (
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/itests/kit2"
)

// TestClient does a basic test to exercise the client CLI commands.
func TestClient(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	kit2.QuietMiningLogs()

	blockTime := 5 * time.Millisecond
	client, _, ens := kit2.EnsembleMinimal(t, kit2.MockProofs(), kit2.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)
	kit2.RunClientTest(t, cli.Commands, *client)
}
