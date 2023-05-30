// stm: #integration
package itests

import (
	"testing"
	"time"

	"github.com/filecoin-project/lotus/itests/kit"
)

func TestDealsWithSealingAndRPC(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.WithAllSubsystems()) // no mock proofs.
	ens.InterconnectAll().BeginMining(250 * time.Millisecond)
	dh := kit.NewDealHarness(t, client, miner, miner)

	t.Run("stdretrieval", func(t *testing.T) {
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1})
	})

	t.Run("fastretrieval", func(t *testing.T) {
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1, FastRetrieval: true})
	})

	t.Run("fastretrieval-twodeals-sequential", func(t *testing.T) {
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1, FastRetrieval: true})
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1, FastRetrieval: true})
	})

	t.Run("stdretrieval-carv1", func(t *testing.T) {
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1, UseCARFileForStorageDeal: true})
	})

}
