package itests

import (
	"testing"
	"time"

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/itests/multisig"
)

// TestMultisig does a basic test to exercise the multisig CLI commands
func TestMultisig(t *testing.T) {

	kit.QuietMiningLogs()

	blockTime := 5 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	multisig.RunMultisigTests(t, client)
}
