// stm: #integration
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
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001
	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	kit.QuietMiningLogs()

	blockTime := 5 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)
	kit.RunClientTest(t, cli.Commands, client)
}
